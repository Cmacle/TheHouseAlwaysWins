// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"bufio"
	"crypto/subtle"
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

	Lobby      *Lobby
	notify     Notifier
	ln         net.Listener
	configPath string
	// set by Main: reload server.json, shut down
	reloadFn func() error
	quitFn   func()
	upd      updateState
	nconn    atomic.Int32
}

type conn struct {
	nc     net.Conn
	srv    *Server
	out    chan []byte
	closed chan struct{}
	once   sync.Once

	user    string
	token   string
	lobbyOK bool // the game knows about the lobby (newer than 1.8)
	fails   int
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
		go guard(s.Log.Printf, "a connection writer", nil, c.writer)
		go func() {
			defer s.nconn.Add(-1)
			guard(s.Log.Printf, "a connection", func() { c.nc.Close() }, c.reader)
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
			s.Lobby.disconnect(c)
		}
	}()
	c.send(proto.SServerInfo, proto.ServerInfo{
		Name: s.cfg().ServerName, Version: proto.Version,
		AllowRegistration: s.cfg().AllowRegistration, MOTD: s.cfg().MOTD,
		NeedsPassword: s.cfg().ServerPassword != "",
		ServerVersion: Version, Updates: s.cfg().OfferClientUpdates,
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
		c.lobbyOK = h.Lobby
		return true
	case proto.CTableBoard:
		var req proto.TableBoardReq
		_ = decode(m, &req)
		tb := proto.TableBoard{Table: req.Table, Entries: s.Store.TableBoard(req.Table, 100)}
		for _, ti := range s.Lobby.Tables() {
			if ti.ID == req.Table {
				tb.Name = ti.Name
			}
		}
		if tb.Entries == nil {
			tb.Entries = []proto.TableEntry{}
		}
		c.send(proto.STableBoard, tb)
		return true
	case proto.CLeaderboard:
		var req proto.LeaderboardReq
		_ = decode(m, &req)
		by := SortNet
		switch req.Sort {
		case SortBalance, SortPeak:
			by = req.Sort
		}
		c.send(proto.SLeaderboard, proto.Leaderboard{Sort: req.Sort, Entries: s.Store.Leaderboard(100, by), House: s.Store.House()})
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
			if want := s.cfg().ServerPassword; want != "" &&
				subtle.ConstantTimeCompare([]byte(r.ServerPassword), []byte(want)) != 1 {
				reason := "Wrong server password. Ask whoever runs the server for it."
				if r.ServerPassword == "" {
					reason = "This server needs a server password to create an account."
				}
				s.Log.Printf("registration refused for %q: bad server password", r.Username)
				time.Sleep(time.Second) // slow down guessing
				c.send(proto.SAuthFail, proto.AuthFail{Reason: reason})
				return true
			}
			prof, tok, err := s.Store.Register(r.Username, r.Password, r.Avatar, s.cfg().RefillAmount, s.cfg().RefillPeriod(time.Now()))
			if err != nil {
				c.send(proto.SAuthFail, proto.AuthFail{Reason: err.Error()})
				return true
			}
			s.Log.Printf("new account: %s", prof.Username)
			c.authed(prof, tok)
			s.notifier().LeaderboardChanged()
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
	case proto.CBet, proto.CAction, proto.CSitOut, proto.CChat, proto.CPoker, proto.CDice, proto.CCasino:
		s.Lobby.route(c, m)
	case proto.CGiftInfo:
		var gi proto.GiftInfo
		if decode(m, &gi) == nil {
			go s.Lobby.giftInfo(c, gi.To)
		}
	case proto.CGift:
		var gf proto.Gift
		if decode(m, &gf) == nil {
			go s.Lobby.gift(c, gf.To, gf.Amount)
		}
	case proto.CJoinTable:
		var j proto.JoinTable
		if decode(m, &j) == nil {
			s.Lobby.join(c, j.ID, j.BuyIn)
		}
	case proto.CLeaveTable:
		s.Lobby.leaveTable(c)
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
		s.Lobby.Broadcast()
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
		s.Lobby.Broadcast()
	case proto.CSettings, proto.CSetSetting:
		if !s.Store.IsAdmin(c.user) {
			c.send(proto.SError, proto.ErrorMsg{Message: "Only admins can change server settings."})
			return true
		}
		var res proto.SettingsList
		var tableID string
		if m.Type == proto.CSetSetting {
			var ss proto.SetSetting
			if decode(m, &ss) != nil {
				return true
			}
			tableID = ss.Table
			var err error
			if ss.Table != "" {
				err = s.Lobby.SetTableSetting(ss.Table, ss.Key, ss.Value)
			} else {
				err = s.SetSetting(ss.Key, ss.Value)
			}
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Changed = ss.Key
				s.Log.Printf("admin %s set %s %s", c.user, ss.Table, ss.Key)
			}
		} else {
			var rq proto.SettingsReq
			_ = decode(m, &rq)
			tableID = rq.Table
		}
		res.Table = tableID
		if tableID != "" {
			list, err := s.Lobby.TableSettings(tableID)
			if err != nil {
				res.Error = err.Error()
			}
			res.Settings = list
		} else {
			res.Settings = s.SettingInfos(true)
		}
		c.send(proto.SSettings, res)
	case proto.CLogout:
		s.Store.RevokeToken(c.user, c.token)
		return false
	case proto.CRecords:
		c.send(proto.SRecords, s.Store.Records())
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
	c.srv.Lobby.authed(c)
}
