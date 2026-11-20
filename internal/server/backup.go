// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

// Backups are zip files in <data_dir>/backups holding accounts.json, invites.json,
// house.json, the avatars and the server config. One is made when the server
// starts, every backup_interval_minutes while anything changed, before risky
// console commands, after a crash is caught, and on shutdown. The newest
// backup_keep are kept, plus one a day for backup_days days.

const backupPrefix = "backup-"

// backupDir is where backups go.
func (s *Store) backupDir() string { return filepath.Join(s.dir, "backups") }

// Backup writes a backup zip now. label goes in the file name ("startup",
// "hourly", "shutdown"...). configPath is the server config to include.
func (s *Store) Backup(label, configPath string) (string, error) {
	// make sure what's on disk is current first
	if err := s.Save(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.backupDir(), 0o755); err != nil {
		return "", err
	}
	name := backupPrefix + time.Now().Format("2006-01-02_150405") + "-" + label + ".zip"
	path := filepath.Join(s.backupDir(), name)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)
	add := func(src, as string) error {
		in, err := os.Open(src)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer in.Close()
		st, _ := in.Stat()
		h := &zip.FileHeader{Name: as, Method: zip.Deflate}
		if st != nil {
			h.Modified = st.ModTime()
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	}
	err = add(filepath.Join(s.dir, "accounts.json"), "accounts.json")
	if err == nil {
		err = add(filepath.Join(s.dir, "house.json"), "house.json")
	}
	if _, serr := os.Stat(filepath.Join(s.dir, "invites.json")); err == nil && serr == nil {
		err = add(filepath.Join(s.dir, "invites.json"), "invites.json")
	}
	if err == nil && configPath != "" {
		err = add(configPath, "server.json")
	}
	if err == nil {
		ents, _ := os.ReadDir(filepath.Join(s.dir, "avatars"))
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
				continue
			}
			if err = add(filepath.Join(s.dir, "avatars", e.Name()), "avatars/"+e.Name()); err != nil {
				break
			}
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	s.backedUp = s.changes
	return name, nil
}

// changedSinceBackup reports whether anything was saved since the last backup.
func (s *Store) changedSinceBackup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changes != s.backedUp || s.dirty
}

// Backups lists backup files, newest first.
func (s *Store) Backups() []string {
	ents, _ := os.ReadDir(s.backupDir())
	var out []string
	for _, e := range ents {
		if n := e.Name(); strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, ".zip") {
			out = append(out, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// backupTime reads the time out of a backup's file name.
func backupTime(name string) (time.Time, bool) {
	s := strings.TrimPrefix(name, backupPrefix)
	if len(s) < len("2006-01-02_150405") {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02_150405", s[:17], time.Local)
	return t, err == nil
}

// PruneBackups deletes old backups: it keeps the newest keep, plus the newest
// one from each of the last days days.
func (s *Store) PruneBackups(keep, days int) (removed int) {
	list := s.Backups() // newest first
	if keep < 1 {
		keep = 1
	}
	keepSet := map[string]bool{}
	for i, n := range list {
		if i < keep {
			keepSet[n] = true
		}
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	seenDay := map[string]bool{}
	for _, n := range list {
		t, ok := backupTime(n)
		if !ok {
			keepSet[n] = true // not ours to judge
			continue
		}
		day := t.Format("2006-01-02")
		if t.After(cutoff) && !seenDay[day] {
			seenDay[day] = true
			keepSet[n] = true
		}
	}
	for _, n := range list {
		if !keepSet[n] {
			if os.Remove(filepath.Join(s.backupDir(), n)) == nil {
				removed++
			}
		}
	}
	return removed
}

// Restore unpacks a backup over the data folder (the config is restored as
// server.json.restored next to the real one, for you to look at). The current
// data is backed up first. Saving stops afterwards so the running server
// can't overwrite what was restored: restart it to load the backup.
func (s *Store) Restore(name, configPath string) error {
	name = filepath.Base(name)
	src := filepath.Join(s.backupDir(), name)
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if _, err := s.Backup("before-restore", configPath); err != nil {
		return fmt.Errorf("couldn't back up the current data first: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range zr.File {
		dst := ""
		switch {
		case f.Name == "accounts.json" || f.Name == "house.json" || f.Name == "invites.json":
			dst = filepath.Join(s.dir, f.Name)
		case strings.HasPrefix(f.Name, "avatars/") && !strings.Contains(f.Name[8:], "/") && !strings.Contains(f.Name, ".."):
			dst = filepath.Join(s.dir, "avatars", f.Name[8:])
		case f.Name == "server.json" && configPath != "":
			dst = configPath + ".restored"
		default:
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return err
		}
		if err := writeFileSafe(dst, b, false); err != nil {
			return err
		}
	}
	s.frozen = true
	return nil
}

// loadWithFallback reads a data file. If it's missing or won't parse, it
// tries the copy kept from the previous save (.bak), then the newest backup
// that has a good copy. A bad file is kept as name.corrupt-<time>.
func loadWithFallback(dir, name string, parse func([]byte) error, logf func(string, ...any)) (found bool, err error) {
	path := filepath.Join(dir, name)
	b, rerr := os.ReadFile(path)
	if rerr == nil {
		perr := parse(b)
		if perr == nil {
			return true, nil
		}
		bad := path + ".corrupt-" + time.Now().Format("20060102-150405")
		os.Rename(path, bad)
		logf("WARNING: %s is damaged (%v); kept it as %s and looking for a good copy", name, perr, filepath.Base(bad))
	} else if !errors.Is(rerr, fs.ErrNotExist) {
		return false, rerr
	}
	// the copy from the save before the last one
	if b, err := os.ReadFile(path + ".bak"); err == nil && parse(b) == nil {
		logf("WARNING: loaded %s from %s.bak (the save before last)", name, name)
		writeFileSafe(path, b, false)
		return true, nil
	}
	if rerr != nil && errors.Is(rerr, fs.ErrNotExist) {
		// nothing there at all: a new server, unless a backup says otherwise
		// and the file was clearly lost (the .bak exists but was bad)
		if _, err := os.Stat(path + ".bak"); err != nil {
			return false, nil
		}
	}
	tmp := &Store{dir: dir}
	for _, bn := range tmp.Backups() {
		zr, err := zip.OpenReader(filepath.Join(tmp.backupDir(), bn))
		if err != nil {
			continue
		}
		for _, f := range zr.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				break
			}
			b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
			rc.Close()
			if err == nil && parse(b) == nil {
				zr.Close()
				logf("WARNING: restored %s from backup %s", name, bn)
				writeFileSafe(path, b, false)
				return true, nil
			}
		}
		zr.Close()
	}
	return false, fmt.Errorf("%s is damaged and no good copy was found in .bak or %s", name, tmp.backupDir())
}

// writeFileSafe writes a file so a crash or power cut can't leave it half
// written: it writes a temp file, flushes it to disk and renames it into
// place. With keepPrev, the old version is kept as name.bak first.
func writeFileSafe(path string, b []byte, keepPrev bool) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if keepPrev {
		if _, err := os.Stat(path); err == nil {
			// a hard link keeps path in place the whole time; fall back to copying
			os.Remove(path + ".bak")
			if os.Link(path, path+".bak") != nil {
				if old, err := os.ReadFile(path); err == nil {
					os.WriteFile(path+".bak", old, 0o644)
				}
			}
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

// syncDir flushes a directory entry to disk (a no-op where that isn't
// possible, like Windows).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// guard runs f and turns a panic into a logged error with a stack trace, so
// one bug can't take the whole server down. onPanic runs after a catch.
func guard(logf func(string, ...any), what string, onPanic func(), f func()) {
	defer func() {
		if r := recover(); r != nil {
			logf("CRASH caught in %s: %v\n%s", what, r, debug.Stack())
			if onPanic != nil {
				onPanic()
			}
		}
	}()
	f()
}
