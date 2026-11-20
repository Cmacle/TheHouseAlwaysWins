// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Invite codes: with require_invite on, making an account needs a code an
// admin generated. Each code works once, so nobody can make a pile of
// accounts on their own. Codes are four characters from an alphabet without
// look-alikes (no 0/O, 1/I/L), case doesn't matter, and they live in
// invites.json next to accounts.json (and in backups).

const inviteAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// InviteLen is how long a code is.
const InviteLen = 4

// Invite is an unused invite code.
type Invite struct {
	Code    string    `json:"code"`
	By      string    `json:"by"` // the admin (or "server") who made it
	Note    string    `json:"note,omitempty"`
	Created time.Time `json:"created"`
}

// errInvite marks a refused invite code (as opposed to a bad username or
// password), so guesses can be slowed down.
var errInvite = errors.New("invite code")

type inviteErr struct{ msg string }

func (e inviteErr) Error() string        { return e.msg }
func (e inviteErr) Is(target error) bool { return target == errInvite }

// NormalizeInvite tidies a typed code: upper case, no spaces or dashes.
func NormalizeInvite(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.NewReplacer(" ", "", "-", "").Replace(code)
}

func newInviteCode() string {
	b := make([]byte, InviteLen)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(inviteAlphabet))))
		b[i] = inviteAlphabet[n.Int64()]
	}
	return string(b)
}

// MakeInvites creates n new codes.
func (s *Store) MakeInvites(n int, by, note string) []string {
	n = min(max(n, 1), 50)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invites == nil {
		s.invites = map[string]Invite{}
	}
	var out []string
	for len(out) < n {
		c := newInviteCode()
		if _, taken := s.invites[c]; taken {
			continue
		}
		s.invites[c] = Invite{Code: c, By: by, Note: note, Created: time.Now()}
		out = append(out, c)
	}
	s.dirty = true
	return out
}

// Invites lists the unused codes, oldest first.
func (s *Store) Invites() []Invite {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Invite, 0, len(s.invites))
	for _, iv := range s.invites {
		out = append(out, iv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}

// RevokeInvite deletes an unused code ("all" deletes every one) and says how
// many went.
func (s *Store) RevokeInvite(code string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	if strings.EqualFold(code, "all") {
		n = len(s.invites)
		s.invites = map[string]Invite{}
	} else if _, ok := s.invites[NormalizeInvite(code)]; ok {
		delete(s.invites, NormalizeInvite(code))
		n = 1
	}
	if n > 0 {
		s.dirty = true
	}
	return n
}

// RegisterInvited makes an account with an invite code, using the code up.
// The code is only spent if the account is made.
func (s *Store) RegisterInvited(code, username, password string, avatar []byte, startBalance int64, period string) (string, *Invite, error) {
	code = NormalizeInvite(code)
	s.mu.Lock()
	iv, ok := s.invites[code]
	if ok {
		delete(s.invites, code) // hold it so two sign-ups can't share it
	}
	s.mu.Unlock()
	if !ok {
		if code == "" {
			return "", nil, inviteErr{"This server needs an invite code to make an account. Ask an admin for one."}
		}
		return "", nil, inviteErr{"That invite code isn't valid (or was already used)."}
	}
	prof, tok, err := s.Register(username, password, avatar, startBalance, period)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.invites[code] = iv // give it back
		return "", nil, err
	}
	if a := s.accounts[strings.ToLower(prof.Username)]; a != nil {
		a.InvitedBy = iv.By
		a.InviteCode = iv.Code
	}
	s.dirty = true
	return tok, &iv, nil
}

func (s *Store) loadInvites() {
	var list []Invite
	if found, err := loadWithFallback(s.dir, "invites.json", func(b []byte) error {
		list = nil
		return json.Unmarshal(b, &list)
	}, s.logf); err != nil {
		s.logf("WARNING: %v; no invite codes loaded", err)
	} else if !found {
		return
	}
	s.invites = map[string]Invite{}
	for _, iv := range list {
		if c := NormalizeInvite(iv.Code); len(c) == InviteLen {
			iv.Code = c
			s.invites[c] = iv
		}
	}
}

// invitesJSON is the codes as saved; call with s.mu held.
func (s *Store) invitesJSON() []byte {
	list := make([]Invite, 0, len(s.invites))
	for _, iv := range s.invites {
		list = append(list, iv)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Code < list[j].Code })
	b, _ := json.MarshalIndent(list, "", "  ")
	return b
}

func (s *Store) invitesPath() string { return filepath.Join(s.dir, "invites.json") }

// inviteLines is the invite list for the console.
func inviteLines(list []Invite) []string {
	if len(list) == 0 {
		return []string{"  no unused invite codes ('invite' makes one)"}
	}
	out := []string{fmt.Sprintf("  %d unused invite code(s):", len(list))}
	for _, iv := range list {
		l := fmt.Sprintf("  %s  made by %s %s", iv.Code, iv.By, iv.Created.Format("2006-01-02 15:04"))
		if iv.Note != "" {
			l += "  (" + iv.Note + ")"
		}
		out = append(out, l)
	}
	return out
}
