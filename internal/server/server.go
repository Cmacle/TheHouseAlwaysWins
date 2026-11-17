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
	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	s := &Server{Store: store, Log: lg}
	s.cfgp.Store(cfg)
	s.Game = NewGame(cfg, store, nil, lg)
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
func Main(configPath string, interactive bool) error {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(abs)
	cfg, created, err := LoadConfig(abs)
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
	lg := log.New(out, "", log.LstdFlags)
	lg.Printf("The House Always Wins server %s", Version)
	if created {
		lg.Printf("created default config at %s - edit it and restart (or type 'reload')", abs)
	} else {
		lg.Printf("config: %s", abs)
	}

	s, err := New(cfg, baseDir, lg)
	if err != nil {
		return err
	}
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

	// periodic save
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if err := s.Store.Save(); err != nil {
					lg.Printf("save failed: %v", err)
				}
			}
		}
	}()

	quit := make(chan struct{}, 1)
	if interactive {
		go s.RunConsole(os.Stdin, os.Stdout, func() error {
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
		}, func() { quit <- struct{}{} })
	}

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
	lg.Printf("bye")
	return nil
}
