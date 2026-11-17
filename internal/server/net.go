package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Server ties the listener, store and game together.
type Server struct {
	cfgp  atomic.Pointer[Config]
	Store *Store
	Game  *Game
	Log   *log.Logger

	ln    net.Listener
	nconn atomic.Int32
}

type conn struct {
	nc     net.Conn
	srv    *Server
	out    chan []byte
	closed chan struct{}
	once   sync.Once

	user  string
	token string
	fails int
}

func (s *Server) cfg() *Config { return s.cfgp.Load() }

func decode(m proto.Msg, v any) error {
	if len(m.Data) == 0 {
		return errors.New("empty")
	}
	return json.Unmarshal(m.Data, v)
}

func (c *conn) send(t string, v any) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.out <- proto.Encode(t, v):
	default:
		// client is too slow to keep up; drop it
		c.close()
	}
}

func (c *conn) event(e proto.Event) { c.send(proto.SEvent, e) }

func (c *conn) close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.nc.Close()
	})
}

func (c *conn) kick(reason string) {
	c.send(proto.SKicked, proto.ErrorMsg{Message: reason})
	time.AfterFunc(300*time.Millisecond, c.close)
}

// Listen opens the TCP listener.
func (s *Server) Listen() error {
	addr := net.JoinHostPort(s.cfg().ListenAddress, strconv.Itoa(s.cfg().Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.Log.Printf("listening for players on %s", ln.Addr())
	return nil
}

// Addr returns the listener address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Serve accepts connections until the listener is closed.
func (s *Server) Serve() {
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.Log.Printf("accept: %v", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		limit := int32(s.cfg().MaxPlayers + s.cfg().MaxSpectators + 16)
		if s.nconn.Load() >= limit {
			_, _ = nc.Write(proto.Encode(proto.SKicked, proto.ErrorMsg{Message: "Server is full."}))
			_ = nc.Close()
			continue
		}
		s.nconn.Add(1)
		c := &conn{nc: nc, srv: s, out: make(chan []byte, 512), closed: make(chan struct{})}
		go c.writer()
		go func() {
			c.reader()
			s.nconn.Add(-1)
		}()
	}
}

// Close stops accepting connections.
func (s *Server) Close() {
	if s.ln != nil {
		_ = s.ln.Close()
	}
}

func (c *conn) writer() {
	w := bufio.NewWriter(c.nc)
	for {
		select {
		case <-c.closed:
			return
		case b := <-c.out:
			_ = c.nc.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if _, err := w.Write(b); err != nil {
				c.close()
				return
			}
			// batch whatever else is queued
			for n := len(c.out); n > 0; n-- {
				if _, err := w.Write(<-c.out); err != nil {
					c.close()
					return
				}
			}
			if err := w.Flush(); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *conn) reader() {
	s := c.srv
	defer func() {
		c.close()
		if c.user != "" {
			s.Game.events <- event{kind: evLeave, c: c}
		}
	}()
	c.send(proto.SServerInfo, proto.ServerInfo{
		Name: s.cfg().ServerName, Version: proto.Version,
		AllowRegistration: s.cfg().AllowRegistration, MOTD: s.cfg().MOTD,
	})
	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64*1024), proto.MaxLine)
	for {
		_ = c.nc.SetReadDeadline(time.Now().Add(90 * time.Second))
		if !sc.Scan() {
			return
		}
		var m proto.Msg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		if !c.handle(m) {
			return
		}
	}
}

func (c *conn) handle(m proto.Msg) bool {
	s := c.srv
	switch m.Type {
	case proto.CPing:
		c.send(proto.SPong, nil)
		return true
	case proto.CHello:
		var h proto.Hello
		_ = decode(m, &h)
		if h.Version != proto.Version {
			c.send(proto.SAuthFail, proto.AuthFail{Reason: "Client version mismatch - please update your game."})
			time.Sleep(300 * time.Millisecond)
			return false
		}
		return true
	case proto.CLeaderboard:
		var req proto.LeaderboardReq
		_ = decode(m, &req)
		by := SortNet
		if req.Sort == SortBalance {
			by = SortBalance
		}
		c.send(proto.SLeaderboard, proto.Leaderboard{Sort: by, Entries: s.Store.Leaderboard(100, by)})
		return true
	case proto.CAvatarGet:
		var a proto.AvatarGet
		if decode(m, &a) == nil {
			data, ver := s.Store.Avatar(a.Username)
			c.send(proto.SAvatar, proto.Avatar{Username: a.Username, Version: ver, Data: data})
		}
		return true
	}

	if c.user == "" {
		switch m.Type {
		case proto.CRegister:
			var r proto.Register
			if decode(m, &r) != nil {
				return false
			}
			if !s.cfg().AllowRegistration {
				c.send(proto.SAuthFail, proto.AuthFail{Reason: "Registration is closed on this server."})
				return true
			}
			prof, tok, err := s.Store.Register(r.Username, r.Password, r.Avatar, s.cfg().RefillAmount, s.cfg().RefillPeriod(time.Now()))
			if err != nil {
				c.send(proto.SAuthFail, proto.AuthFail{Reason: err.Error()})
				return true
			}
			s.Log.Printf("new account: %s", prof.Username)
			c.authed(prof, tok)
			s.Game.notify.LeaderboardChanged()
		case proto.CLogin:
			var l proto.Login
			if decode(m, &l) != nil {
				return false
			}
			prof, tok, err := s.Store.Login(l.Username, l.Password, l.Token)
			if err != nil {
				c.fails++
				c.send(proto.SAuthFail, proto.AuthFail{Reason: err.Error()})
				return c.fails < 8
			}
			c.authed(prof, tok)
		default:
			c.send(proto.SError, proto.ErrorMsg{Message: "Please log in first."})
		}
		return true
	}

	switch m.Type {
	case proto.CBet, proto.CAction, proto.CSitOut:
		s.Game.events <- event{kind: evMsg, c: c, msg: m}
	case proto.CAvatarSet:
		var a proto.AvatarSet
		if decode(m, &a) != nil {
			return true
		}
		if _, err := s.Store.SetAvatar(c.user, a.Data); err != nil {
			c.send(proto.SError, proto.ErrorMsg{Message: err.Error()})
			return true
		}
		if p, ok := s.Store.Profile(c.user); ok {
			c.send(proto.SProfile, p)
		}
		s.Game.Broadcast()
	case proto.CEquip:
		var e proto.Equip
		if decode(m, &e) != nil {
			return true
		}
		if err := s.Store.Equip(c.user, e.Kind, e.ID); err != nil {
			c.send(proto.SError, proto.ErrorMsg{Message: err.Error()})
			return true
		}
		if p, ok := s.Store.Profile(c.user); ok {
			c.send(proto.SProfile, p)
		}
		s.Game.Broadcast()
	case proto.CLogout:
		s.Store.RevokeToken(c.user, c.token)
		return false
	case proto.CProfile:
		if p, ok := s.Store.Profile(c.user); ok {
			c.send(proto.SProfile, p)
		}
	}
	return true
}

func (c *conn) authed(prof *proto.Profile, tok string) {
	c.user, c.token = prof.Username, tok
	c.send(proto.SAuthOK, proto.AuthOK{Username: prof.Username, Token: tok, Profile: *prof})
	c.srv.Game.events <- event{kind: evJoin, c: c, user: prof.Username}
}
