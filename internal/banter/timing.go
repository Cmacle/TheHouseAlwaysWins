package banter

import (
	"strings"
	"time"
)

// Words splits a line into the words the dealer "says" one at a time.
func Words(text string) []string { return strings.Fields(text) }

// WordDelay is how long the dealer takes to say one word: longer words take
// longer, and he pauses after punctuation.
func WordDelay(word string) time.Duration {
	d := 70*time.Millisecond + time.Duration(len(word))*22*time.Millisecond
	switch word[len(word)-1] {
	case '.', '!', '?':
		d += 180 * time.Millisecond
	case ',', ';', ':':
		d += 90 * time.Millisecond
	}
	return d
}

// SpeakTime is how long it takes to say the whole line.
func SpeakTime(text string) time.Duration {
	var t time.Duration
	for _, w := range Words(text) {
		t += WordDelay(w)
	}
	return t
}
