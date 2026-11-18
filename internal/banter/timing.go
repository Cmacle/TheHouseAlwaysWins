// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

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

// Page is one bubble's worth of a line. Long lines and stories are said a
// page at a time.
type Page struct {
	Text string
	Para bool // starts a new paragraph (not set on the very first page)
}

// PageChars is roughly what fits in the speech bubble (four lines).
const PageChars = 118

// Pages splits text into bubble-sized pages. Paragraphs (separated by a blank
// line) always start a new page; inside a paragraph it breaks at the end of
// a sentence when it can, and keeps pages about the same size.
func Pages(text string) []Page {
	var out []Page
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		total := len(strings.Join(words, " "))
		n := (total + PageChars - 1) / PageChars
		target := total / n
		first := true
		for len(words) > 0 {
			end, size, sentEnd := 0, 0, 0
			for end < len(words) {
				add := len(words[end])
				if end > 0 {
					add++
				}
				if size+add > PageChars && end > 0 {
					break
				}
				size += add
				end++
				if endsSentence(words[end-1]) {
					sentEnd = end
					if size >= target {
						break
					}
				}
			}
			// overflowed mid-sentence: back up to the last full sentence if
			// that leaves a decent page
			if end < len(words) && sentEnd > 0 && sentEnd < end && len(strings.Join(words[:sentEnd], " ")) >= PageChars/2 {
				end = sentEnd
			}
			out = append(out, Page{Text: strings.Join(words[:end], " "), Para: first && len(out) > 0})
			words = words[end:]
			first = false
		}
	}
	return out
}

func endsSentence(w string) bool {
	w = strings.TrimRight(w, `"')`)
	return w != "" && strings.ContainsRune(".!?", rune(w[len(w)-1]))
}
