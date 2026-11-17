package client

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// NetClient is a connection to the game server.
type NetClient struct {
	Addr string
	conn net.Conn
	in   chan proto.Msg
	mu   sync.Mutex
	w    *bufio.Writer
	dead chan struct{}
	once sync.Once
}

// Dial connects to host:port (default port 7777).
func Dial(addr string) (*NetClient, error) {
	addr = strings.TrimSpace(addr)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "7777")
	}
	c, err := net.DialTimeout("tcp", addr, 6*time.Second)
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	n := &NetClient{Addr: addr, conn: c, in: make(chan proto.Msg, 512), w: bufio.NewWriter(c), dead: make(chan struct{})}
	go n.reader()
	n.Send(proto.CHello, proto.Hello{Version: proto.Version})
	return n, nil
}

func (n *NetClient) reader() {
	defer n.Close()
	sc := bufio.NewScanner(n.conn)
	sc.Buffer(make([]byte, 64*1024), proto.MaxLine)
	for sc.Scan() {
		var m proto.Msg
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			select {
			case n.in <- m:
			case <-n.dead:
				return
			}
		}
	}
}

// Send writes a message.
func (n *NetClient) Send(t string, v any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	select {
	case <-n.dead:
		return
	default:
	}
	_ = n.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := n.w.Write(proto.Encode(t, v)); err == nil {
		err = n.w.Flush()
		if err != nil {
			go n.Close()
		}
	}
}

// Close closes the connection.
func (n *NetClient) Close() {
	n.once.Do(func() {
		close(n.dead)
		_ = n.conn.Close()
	})
}

// Dead is closed when the connection ends.
func (n *NetClient) Dead() <-chan struct{} { return n.dead }

// Poll returns queued messages without blocking.
func (n *NetClient) Poll() []proto.Msg {
	var out []proto.Msg
	for {
		select {
		case m := <-n.in:
			out = append(out, m)
		default:
			return out
		}
	}
}
