// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import "testing"

func TestServerAddr(t *testing.T) {
	cases := []struct{ host, port, want string }{
		{"play.example.com", "7777", "play.example.com:7777"},
		{"play.example.com", "", "play.example.com:7777"},
		{"play.example.com", "9000", "play.example.com:9000"},
		{"https://play.example.com/", "8000", "play.example.com:8000"},
		{"play.example.com:9001", "", "play.example.com:9001"},
		{"play.example.com:9001", "7777", "play.example.com:9001"},
		{"  203.0.113.7 ", "7777", "203.0.113.7:7777"},
		{"::1", "7777", "[::1]:7777"},
	}
	for _, c := range cases {
		if got := joinServer(c.host, c.port); got != c.want {
			t.Errorf("joinServer(%q,%q) = %q, want %q", c.host, c.port, got, c.want)
		}
	}
	if h, p := splitServer("example.com:1234"); h != "example.com" || p != "1234" {
		t.Errorf("split %s %s", h, p)
	}
	if normAddr("example.com") != "example.com:7777" {
		t.Error("normAddr default port")
	}
}
