// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package discord

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serverFrame(fin bool, op byte, payload []byte) []byte {
	b0 := op
	if fin {
		b0 |= 0x80
	}
	out := []byte{b0}
	n := len(payload)
	switch {
	case n < 126:
		out = append(out, byte(n))
	case n <= 0xffff:
		out = append(out, 126, byte(n>>8), byte(n))
	default:
		out = append(out, 127)
		out = binary.BigEndian.AppendUint64(out, uint64(n))
	}
	return append(out, payload...)
}

func TestWebsocket(t *testing.T) {
	big := strings.Repeat("x", 70000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Sec-WebSocket-Key")
		h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		hj, _ := w.(http.Hijacker)
		c, rw, _ := hj.Hijack()
		defer c.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
			base64.StdEncoding.EncodeToString(h[:]) + "\r\n\r\n")
		rw.Write(serverFrame(false, opText, []byte("hel")))
		rw.Write(serverFrame(true, opPing, []byte("p")))
		rw.Write(serverFrame(true, opCont, []byte("lo")))
		rw.Write(serverFrame(true, opText, []byte(big)))
		rw.Flush()
		// read client frames: expect pong then masked text
		br := bufio.NewReader(rw)
		for i := 0; i < 2; i++ {
			var hdr [2]byte
			io.ReadFull(br, hdr[:])
			n := int(hdr[1] & 0x7f)
			var mask [4]byte
			io.ReadFull(br, mask[:])
			p := make([]byte, n)
			io.ReadFull(br, p)
			for j := range p {
				p[j] ^= mask[j%4]
			}
			if hdr[0]&0x0f == opText {
				rw.Write(serverFrame(true, opText, p))
			}
		}
		rw.Write(serverFrame(true, opClose, []byte{0x0f, 0xa4, 'b', 'y', 'e'}))
		rw.Flush()
		br.ReadByte()
	}))
	defer srv.Close()
	ws, err := dialWS("ws" + strings.TrimPrefix(srv.URL, "http") + "/?v=10")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ws.ReadMessage()
	if err != nil || string(m) != "hello" {
		t.Fatalf("%q %v", m, err)
	}
	m, err = ws.ReadMessage()
	if err != nil || len(m) != 70000 {
		t.Fatalf("big: %d %v", len(m), err)
	}
	ws.WriteText([]byte("echo"))
	m, err = ws.ReadMessage()
	if err != nil || string(m) != "echo" {
		t.Fatalf("%q %v", m, err)
	}
	_, err = ws.ReadMessage()
	ce, ok := err.(*closeError)
	if !ok || ce.Code != 4004 {
		t.Fatalf("close: %v", err)
	}
}
