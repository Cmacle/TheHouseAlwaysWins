// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func newTestStore(t *testing.T) (*Store, string) {
	dir := t.TempDir()
	st, err := OpenStoreLog(dir, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Register("alice", "pw123456", nil, 1000, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	return st, dir
}

func TestBackupAndRecoverDamagedFiles(t *testing.T) {
	st, dir := newTestStore(t)
	cfgPath := filepath.Join(dir, "server.json")
	os.WriteFile(cfgPath, []byte(`{"port": 7777}`), 0o644)
	name, err := st.Backup("test", cfgPath)
	if err != nil || len(st.Backups()) != 1 || st.Backups()[0] != name {
		t.Fatalf("backup: %v %v", err, st.Backups())
	}
	if st.changedSinceBackup() {
		t.Fatal("should be up to date right after a backup")
	}

	// a second save leaves the first as .bak
	st.Refund("alice", 500)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "accounts.json.bak")); err != nil {
		t.Fatal("no .bak kept")
	}

	// damage accounts.json: the .bak is used (balance from the save before)
	os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(`{"broken`), 0o644)
	st2, err := OpenStoreLog(dir, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if b := st2.Balance("alice"); b != 1000 {
		t.Fatalf("balance after .bak recovery: %d", b)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "accounts.json.corrupt-*"))
	if len(matches) != 1 {
		t.Fatal("the damaged file wasn't kept aside")
	}

	// damage both: the backup zip is used
	os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(`nope`), 0o644)
	os.WriteFile(filepath.Join(dir, "accounts.json.bak"), []byte(`nope`), 0o644)
	st3, err := OpenStoreLog(dir, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st3.Profile("alice"); !ok {
		t.Fatal("account not recovered from the backup")
	}
}

func TestRestoreFreezesSaving(t *testing.T) {
	st, dir := newTestStore(t)
	name, err := st.Backup("one", "")
	if err != nil {
		t.Fatal(err)
	}
	st.Refund("alice", 4000)
	st.Save()
	time.Sleep(1100 * time.Millisecond) // a different file name for the pre-restore backup
	if err := st.Restore(name, ""); err != nil {
		t.Fatal(err)
	}
	st.Refund("alice", 1) // would be saved if not frozen
	st.Save()
	st2, _ := OpenStoreLog(dir, t.Logf)
	if b := st2.Balance("alice"); b != 1000 {
		t.Fatalf("after restore balance is %d, want 1000", b)
	}
	if len(st.Backups()) != 2 {
		t.Fatalf("expected a before-restore backup too: %v", st.Backups())
	}
}

func TestPruneBackups(t *testing.T) {
	st, _ := newTestStore(t)
	os.MkdirAll(st.backupDir(), 0o755)
	now := time.Now()
	// 10 an hour apart today and yesterday, plus some from long ago
	for i := 0; i < 10; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		os.WriteFile(filepath.Join(st.backupDir(), backupPrefix+ts.Format("2006-01-02_150405")+"-auto.zip"), nil, 0o644)
	}
	for _, d := range []int{3, 40, 41} {
		ts := now.AddDate(0, 0, -d)
		os.WriteFile(filepath.Join(st.backupDir(), backupPrefix+ts.Format("2006-01-02_150405")+"-auto.zip"), nil, 0o644)
	}
	st.PruneBackups(4, 30)
	left := st.Backups()
	// newest 4, plus the newest of any other day within 30 days (yesterday
	// maybe, and 3 days ago); the 40+ day old ones go
	for _, n := range left {
		ts, _ := backupTime(n)
		if now.Sub(ts) > 31*24*time.Hour {
			t.Fatalf("kept an old backup: %s", n)
		}
	}
	if len(left) < 5 || len(left) > 6 {
		t.Fatalf("kept %d: %v", len(left), left)
	}
}

func TestCrashInHandIsCaught(t *testing.T) {
	g := storyGame(t)
	g.store.Register("alice", "pw123456", nil, 1000, "")
	p := g.seats[0]
	g.store.Debit("alice", 100)
	p.inHand = true
	p.hands = []*bj.Hand{{Bet: 100}}
	g.phase = proto.PhasePlaying
	g.safely("test", func() { panic("boom") })
	if b := g.store.Balance("alice"); b != 1000 {
		t.Fatalf("bet not refunded after crash: %d", b)
	}
	if p.hands != nil || g.timer == nil {
		t.Fatal("hand not called off / next hand not scheduled")
	}
}

func TestBadConfigFallsBackToLastGood(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "server.json")
	if _, _, err := LoadConfig(p); err != nil { // creates it and the lastgood copy
		t.Fatal(err)
	}
	os.WriteFile(p, []byte(`{"port": 7777,,}`), 0o644)
	cfg, _, warn, err := LoadConfigOrLastGood(p)
	if err != nil || cfg == nil || warn == "" {
		t.Fatalf("no fallback: %v %q", err, warn)
	}
}
