// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/discord"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/release"
)

// Version is the server build version.
var Version = "1.0.0"

// New builds a server from a loaded config. dataDir is resolved relative to
// baseDir when it is not absolute.
func New(cfg *Config, baseDir string, lg *log.Logger) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(cfg.DataDir) {
		cfg.DataDir = filepath.Join(baseDir, cfg.DataDir)
	}
	store, err := OpenStoreLog(cfg.DataDir, lg.Printf)
	if err != nil {
		return nil, err
	}
	s := &Server{Store: store, Log: lg}
	s.cfgp.Store(cfg)
	s.Game = NewGame(cfg, store, nil, lg)
	s.Game.exec = s.Exec
	return s, nil
}

// discordData adapts the server for the Discord bot.
type discordData struct{ s *Server }

func (d discordData) ServerName() string { return d.s.cfg().ServerName }
func (d discordData) Leaderboard(n int, rich bool) []proto.Profile {
	if rich {
		return d.s.Store.Leaderboard(n, SortBalance)
	}
	return d.s.Store.Leaderboard(n, SortNet)
}
func (d discordData) Profile(name string) (proto.Profile, bool) { return d.s.Store.Profile(name) }
func (d discordData) House() proto.HouseStats                   { return d.s.Store.House() }
func (d discordData) TableSummary() string {
	phase, seated, names, hand := d.s.Game.Summary()
	if seated == 0 {
		return "The table is empty. The dealer is shuffling chips out of boredom."
	}
	return fmt.Sprintf("**%d/%d** seats taken · hand #%d · %s\nPlaying: %s",
		seated, d.s.cfg().MaxPlayers, hand, phase, strings.Join(names, ", "))
}

// EnableDiscord starts the Discord integration if configured.
func (s *Server) EnableDiscord(stop <-chan struct{}) {
	dc := s.cfg().Discord
	if !dc.Enabled {
		return
	}
	bot := discord.New(discord.Config{
		BotToken:             strings.TrimSpace(dc.BotToken),
		LeaderboardChannelID: strings.TrimSpace(dc.LeaderboardChannelID),
		AnnounceChannelID:    strings.TrimSpace(dc.AnnounceChannelID),
		WebhookURL:           strings.TrimSpace(dc.WebhookURL),
		SlashCommands:        dc.SlashCommands,
		GuildID:              strings.TrimSpace(dc.GuildID),
		LeaderboardSize:      dc.LeaderboardSize,
		UpdateInterval:       time.Duration(dc.UpdateIntervalMinutes) * time.Minute,
		AnnounceRefills:      dc.AnnounceRefills,
		StateFile:            filepath.Join(s.cfg().DataDir, "discord_state.json"),
	}, discordData{s}, s.Log)
	s.Game.notify = bot
	bot.Start(stop)
}

// Main runs the server until interrupted. It is what the server exe calls.
// Options are how Main runs.
type Options struct {
	Console bool      // read commands from stdin
	LogTo   io.Writer // also send the log here (the server window)
	// Started is called once the server is up (from another goroutine).
	Started func(*Server)
}

func Main(configPath string, opt Options) error {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(abs)
	cfg, created, cfgWarn, err := LoadConfigOrLastGood(abs)
	if err != nil {
		return err
	}
	dataDir := cfg.DataDir
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(baseDir, dataDir)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	var out io.Writer = os.Stdout
	if err == nil {
		defer logFile.Close()
		out = io.MultiWriter(os.Stdout, logFile)
	}
	if opt.LogTo != nil {
		out = io.MultiWriter(out, opt.LogTo)
	}
	lg := log.New(out, "", log.LstdFlags)
	lg.Printf("The House Always Wins server %s", Version)
	if created {
		lg.Printf("created default config at %s - edit it and restart (or type 'reload')", abs)
	} else {
		lg.Printf("config: %s", abs)
	}
	if cfgWarn != "" {
		lg.Print(cfgWarn)
	}
	if len(addedKeys) > 0 {
		lg.Printf("added new settings to %s with default values: %s", filepath.Base(abs), strings.Join(addedKeys, ", "))
	}

	s, err := New(cfg, baseDir, lg)
	if err != nil {
		return err
	}
	s.configPath = abs
	s.backup("startup")
	if err := s.Listen(); err != nil {
		return err
	}
	stop := make(chan struct{})
	s.EnableDiscord(stop)
	gameDone := make(chan struct{})
	go func() { s.Game.Run(stop); close(gameDone) }()
	go s.Serve()
	s.StartHTTP()
	lg.Printf("refill: below %d tops up daily at %s (%s); next at %s", cfg.RefillAmount, cfg.RefillTime, cfg.loc, cfg.NextRefill(time.Now()).Format("Mon Jan 2 15:04 MST"))
	lg.Printf("table: %d seats, betting %s, play %s, other hands hidden: %v", cfg.MaxPlayers, cfg.BettingMode, cfg.PlayMode, cfg.HideOtherHands)

	// save every few seconds; back up every backup_interval_minutes
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		lastBackup := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				guard(lg.Printf, "saving", nil, func() {
					if err := s.Store.Save(); err != nil {
						lg.Printf("save failed (will retry): %v", err)
					}
					if m := s.cfg().BackupIntervalMinutes; m > 0 && time.Since(lastBackup) >= time.Duration(m)*time.Minute {
						lastBackup = time.Now()
						if s.Store.changedSinceBackup() {
							s.backup("auto")
						}
					}
				})
			}
		}
	}()

	quit := make(chan struct{}, 1)
	s.reloadFn = func() error {
		nc, _, err := LoadConfig(abs)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(nc.DataDir) {
			nc.DataDir = filepath.Join(baseDir, nc.DataDir)
		}
		s.cfgp.Store(nc)
		s.Game.Reload(nc)
		return nil
	}
	s.quitFn = func() {
		select {
		case quit <- struct{}{}:
		default:
		}
	}
	if opt.Console {
		go s.RunConsole(os.Stdin, os.Stdout)
	}
	if opt.Started != nil {
		go opt.Started(s)
	}
	// look for a newer server now and then, and tell admins
	go func() {
		for {
			if v := s.Latest(false); v != "" && release.Newer(v, Version) {
				lg.Printf("a newer server is out: v%s (type 'update' to install it)", v)
			}
			select {
			case <-stop:
				return
			case <-time.After(6 * time.Hour):
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-quit:
	}
	lg.Printf("shutting down...")
	s.Close()
	close(stop)
	<-gameDone
	if err := s.Store.Save(); err != nil {
		lg.Printf("save failed: %v", err)
	}
	s.backup("shutdown")
	if s.restartPending() {
		s.finishRestart()
	}
	lg.Printf("bye")
	return nil
}

// backup makes a backup and prunes old ones, logging what happened.
func (s *Server) backup(label string) {
	c := s.cfg()
	name, err := s.Store.Backup(label, s.configPath)
	if err != nil {
		s.Log.Printf("backup failed: %v", err)
		return
	}
	removed := s.Store.PruneBackups(c.BackupKeep, c.BackupDays)
	if label != "auto" || removed > 0 {
		s.Log.Printf("backup saved: %s", name)
	}
}
