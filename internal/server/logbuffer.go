// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"strings"
	"sync"
)

// LogBuffer keeps the last lines written to it, for the server window.
type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	first int // number of the first line kept
	part  string
	max   int
}

// NewLogBuffer keeps up to max lines.
func NewLogBuffer(max int) *LogBuffer { return &LogBuffer{max: max} }

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.part + strings.ReplaceAll(string(p), "\r", "")
	parts := strings.Split(s, "\n")
	b.part = parts[len(parts)-1]
	for _, l := range parts[:len(parts)-1] {
		b.lines = append(b.lines, strings.ReplaceAll(l, "\t", "  "))
	}
	if over := len(b.lines) - b.max; over > 0 {
		b.lines = append([]string(nil), b.lines[over:]...)
		b.first += over
	}
	return len(p), nil
}

// Since returns the lines from number from on, and the number to ask for
// next time.
func (b *LogBuffer) Since(from int) ([]string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if from < b.first {
		from = b.first
	}
	i := from - b.first
	if i > len(b.lines) {
		i = len(b.lines)
	}
	return append([]string(nil), b.lines[i:]...), b.first + len(b.lines)
}
