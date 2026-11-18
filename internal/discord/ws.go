// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package discord

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// A minimal RFC 6455 websocket client: enough for the Discord gateway
// (text frames, fragmentation, ping/pong, close), standard library only.

const (
	opCont  = 0x0
	opText  = 0x1
	opBin   = 0x2
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

const maxFrame = 32 << 20

type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

// closeError carries the websocket close code sent by the server.
type closeError struct {
	Code   int
	Reason string
}

func (e *closeError) Error() string { return fmt.Sprintf("websocket closed: %d %s", e.Code, e.Reason) }

func dialWS(raw string) (*wsConn, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	var nc net.Conn
	d := &net.Dialer{Timeout: 15 * time.Second}
	if u.Scheme == "ws" {
		nc, err = d.Dial("tcp", host)
	} else {
		nc, err = tls.DialWithDialer(d, "tcp", host, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	}
	if err != nil {
		return nil, err
	}
	kb := make([]byte, 16)
	_, _ = rand.Read(kb)
	key := base64.StdEncoding.EncodeToString(kb)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + u.Hostname() + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"User-Agent: " + userAgent + "\r\n\r\n"
	_ = nc.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := io.WriteString(nc, req); err != nil {
		nc.Close()
		return nil, err
	}
	br := bufio.NewReaderSize(nc, 64*1024)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		nc.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		nc.Close()
		return nil, fmt.Errorf("websocket handshake: %s", resp.Status)
	}
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(h[:]) {
		nc.Close()
		return nil, errors.New("websocket handshake: bad accept key")
	}
	_ = nc.SetDeadline(time.Time{})
	return &wsConn{c: nc, br: br}, nil
}

func (w *wsConn) Close() error { return w.c.Close() }

// ReadMessage returns the next complete text or binary message.
func (w *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		var hdr [2]byte
		if _, err := io.ReadFull(w.br, hdr[:]); err != nil {
			return nil, err
		}
		fin := hdr[0]&0x80 != 0
		op := hdr[0] & 0x0f
		masked := hdr[1]&0x80 != 0
		n := uint64(hdr[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > maxFrame || uint64(len(msg))+n > maxFrame {
			return nil, errors.New("websocket frame too large")
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch op {
		case opPing:
			_ = w.write(opPong, payload)
			continue
		case opPong:
			continue
		case opClose:
			ce := &closeError{Code: 1005}
			if len(payload) >= 2 {
				ce.Code = int(binary.BigEndian.Uint16(payload[:2]))
				ce.Reason = string(payload[2:])
			}
			_ = w.write(opClose, payload[:min(2, len(payload))])
			return nil, ce
		case opCont, opText, opBin:
			msg = append(msg, payload...)
		default:
			return nil, fmt.Errorf("websocket: unknown opcode %d", op)
		}
		if fin {
			return msg, nil
		}
	}
}

// WriteText sends a text message.
func (w *wsConn) WriteText(b []byte) error { return w.write(opText, b) }

func (w *wsConn) write(op byte, payload []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	buf := make([]byte, 0, len(payload)+14)
	buf = append(buf, 0x80|op)
	n := len(payload)
	switch {
	case n < 126:
		buf = append(buf, 0x80|byte(n))
	case n <= 0xffff:
		buf = append(buf, 0x80|126, byte(n>>8), byte(n))
	default:
		buf = append(buf, 0x80|127)
		buf = binary.BigEndian.AppendUint64(buf, uint64(n))
	}
	var mask [4]byte
	_, _ = rand.Read(mask[:])
	buf = append(buf, mask[:]...)
	start := len(buf)
	buf = append(buf, payload...)
	for i := range payload {
		buf[start+i] ^= mask[i%4]
	}
	_ = w.c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, err := w.c.Write(buf)
	return err
}
