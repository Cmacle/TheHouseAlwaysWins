// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RunConsole reads admin commands from r. It returns when r hits EOF (for
// example when the server runs as a background service with no stdin), which
// does not stop the server.
func (s *Server) RunConsole(r io.Reader, w io.Writer) {
	sc := bufio.NewScanner(r)
	fmt.Fprintln(w, "Type 'help' for console commands.")
	for sc.Scan() {
		s.Exec(sc.Text(), w)
	}
}

// Exec runs one console command, writing its output to w. The server
// console, admins in the game (typing /command in chat) and the server
// window all use it.
func (s *Server) Exec(line string, w io.Writer) {
	f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(f) == 0 {
		return
	}
	cmd, args := strings.ToLower(f[0]), f[1:]
	need := func(n int, usage string) bool {
		if len(args) < n {
			fmt.Fprintln(w, "usage:", usage)
			return false
		}
		return true
	}
	switch cmd {
	case "help", "?":
		fmt.Fprint(w, `commands:
  players                     who is at the table
  leaderboard [n] [rich]      top n by lifetime net (or by chips with "rich")
  stats <user>                account details
  accounts                    list all accounts
  house                       the dealer's lifetime results
  give <user> <amount>        add chips (negative removes); does not change lifetime net
  refill <user|all>           top up to the refill amount now (counts against net, like the daily refill)
  resetstats <user>           zero lifetime stats and set balance to the refill amount
  setpassword <user> <pass>   change a password and sign out saved logins
  delete <user>               delete an account
  ban <user> [reason]         kick and delete an account, and take what the
                              house won and lost against them out of its totals
  kick <user>                 disconnect a player
  invite [n] [note]           make n one-use invite codes (for require_invite)
  invites                     list unused invite codes
  uninvite <code|all>         cancel an unused invite code
  say <message>               message everyone at the table
  admin <user> [on|off]       make someone an admin (they can type /commands in chat)
  admins                      list admins
  get [filter]                show settings (server.json)
  set <setting> <value>       change a setting and reload (e.g. set card_reveal off)
  update [force]              download the newest server from GitHub and restart into it
  tables                      list the tables
  table add <blackjack|holdem> <name>   open a table
  table remove <id>           close a table (everyone goes back to the lobby with their chips)
  table rename <id> <name>    rename a table
  table get <id>              show a table's settings
  table set <id> <setting> <value>   change a table setting (from the next hand)
  reload                      reload server.json (applies at the next hand; port changes need a restart)
  save                        write accounts to disk now
  backup                      make a backup now
  backups                     list backups (newest first)
  restore <backup>            put a backup back (backs up first; restart after)
  quit                        save and shut down
`)
	case "players", "who":
		for _, l := range s.Lobby.PlayerList() {
			fmt.Fprintln(w, " ", l)
		}
	case "leaderboard", "lb", "top":
		n, by := 10, SortNet
		for _, a := range args {
			if v, err := strconv.Atoi(a); err == nil {
				n = v
			} else if strings.HasPrefix(strings.ToLower(a), "r") || strings.HasPrefix(strings.ToLower(a), "b") {
				by = SortBalance
			}
		}
		for i, p := range s.Store.Leaderboard(n, by) {
			fmt.Fprintf(w, "  %3d. %-16s net %+d  balance %d  hands %d\n", i+1, p.Username, p.Net, p.Balance, p.HandsPlayed)
		}
	case "house", "dealer":
		h := s.Store.House()
		fmt.Fprintf(w, "  house net %+d over %d rounds (%d player hands)\n  won %d in %d rounds, lost %d in %d rounds\n  dealer blackjacks %d, dealer busts %d, player busts %d\n  best round %+d, worst round -%d\n",
			h.Net, h.HandsDealt, h.PlayerHands, h.Won, h.RoundsWon, h.Lost, h.RoundsLost, h.DealerBlackjacks, h.DealerBusts, h.PlayerBusts, h.BiggestWin, h.BiggestLoss)
	case "accounts":
		names := s.Store.Usernames()
		fmt.Fprintf(w, "  %d account(s): %s\n", len(names), strings.Join(names, ", "))
	case "stats":
		if !need(1, "stats <user>") {
			return
		}
		p, ok := s.Store.Profile(args[0])
		if !ok {
			fmt.Fprintln(w, "  no such account")
			return
		}
		fmt.Fprintf(w, "  %s  balance %d  net %+d  won %d  lost %d  refills %d\n  hands %d (W%d L%d P%d)  blackjacks %d  biggest win %d  online %v\n",
			p.Username, p.Balance, p.Net, p.Won, p.Lost, p.RefillTotal, p.HandsPlayed, p.HandsWon, p.HandsLost, p.HandsPushed, p.Blackjacks, p.BiggestWin, p.Online)
	case "give":
		if !need(2, "give <user> <amount>") {
			return
		}
		amt, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			fmt.Fprintln(w, "  bad amount")
			return
		}
		bal, ok := s.Store.AdminAdjust(args[0], amt)
		if !ok {
			fmt.Fprintln(w, "  no such account")
			return
		}
		fmt.Fprintf(w, "  %s now has %d\n", args[0], bal)
		s.Lobby.NotifyUser(args[0], fmt.Sprintf("The house adjusted your chips by %+d.", amt))
		s.notifier().LeaderboardChanged()
	case "refill":
		if !need(1, "refill <user|all>") {
			return
		}
		users := []string{args[0]}
		if strings.EqualFold(args[0], "all") {
			users = s.Store.Usernames()
		}
		var done []string
		for _, u := range users {
			if s.Lobby.InHand(u) {
				fmt.Fprintf(w, "  %s is mid-hand, skipped\n", u)
				continue
			}
			if d := s.Store.ForceRefill(u, s.cfg().RefillAmount); d > 0 {
				done = append(done, u)
				fmt.Fprintf(w, "  %s +%d\n", u, d)
				s.Lobby.NotifyUser(u, fmt.Sprintf("Refill: +%d chips", d))
			}
		}
		if len(done) > 0 {
			s.notifier().Refilled(done, s.cfg().RefillAmount)
			s.notifier().LeaderboardChanged()
		}
		s.Lobby.Broadcast()
	case "resetstats":
		if !need(1, "resetstats <user>") {
			return
		}
		if s.Lobby.InHand(args[0]) {
			fmt.Fprintln(w, "  player is mid-hand, try again in a moment")
			return
		}
		s.backup("before-resetstats")
		fmt.Fprintln(w, " ", okText(s.Store.ResetStats(args[0], s.cfg().RefillAmount)))
		s.Lobby.Broadcast()
		s.notifier().LeaderboardChanged()
	case "setpassword":
		if !need(2, "setpassword <user> <password>") {
			return
		}
		fmt.Fprintln(w, " ", okText(s.Store.SetPassword(args[0], args[1])))
	case "delete":
		if !need(1, "delete <user>") {
			return
		}
		s.backup("before-delete")
		s.Lobby.Kick(args[0], "Your account was deleted.")
		fmt.Fprintln(w, " ", okText(s.Store.Delete(args[0])))
		s.notifier().LeaderboardChanged()
	case "ban":
		if !need(1, "ban <user> [reason]") {
			return
		}
		reason := strings.TrimSpace(strings.Join(args[1:], " "))
		s.backup("before-ban")
		msg := "You have been banned from this server."
		if reason != "" {
			msg += " Reason: " + reason
		}
		s.Lobby.Kick(args[0], msg)
		name, house, err := s.Store.Ban(args[0])
		if err != nil {
			fmt.Fprintln(w, " ", err)
			return
		}
		fmt.Fprintf(w, "  %s is banned: the account is deleted, and %s came out of the house's totals.\n", name, signedChips(house))
		s.Log.Printf("banned %s (%s); house net %+d removed", name, reason, house)
		s.Lobby.Announce("[House] " + name + " has been banned.")
		s.notifier().LeaderboardChanged()
	case "invite", "invite-as":
		by := "server"
		if cmd == "invite-as" { // /invite from an admin in the game
			if !need(1, "invite-as <admin> [n] [note]") {
				return
			}
			by, args = args[0], args[1:]
		}
		n := 1
		if len(args) > 0 {
			if v, err := strconv.Atoi(args[0]); err == nil {
				n, args = v, args[1:]
			}
		}
		codes := s.Store.MakeInvites(n, by, strings.Join(args, " "))
		s.Store.Save()
		s.Log.Printf("%s made %d invite code(s)", by, len(codes))
		fmt.Fprintf(w, "  invite code(s), one account each: %s\n", strings.Join(codes, "  "))
		if !s.cfg().RequireInvite {
			fmt.Fprintln(w, "  (require_invite is off, so anyone can sign up without one: 'set require_invite on')")
		}
	case "invites":
		for _, l := range inviteLines(s.Store.Invites()) {
			fmt.Fprintln(w, l)
		}
	case "uninvite":
		if !need(1, "uninvite <code|all>") {
			return
		}
		n := s.Store.RevokeInvite(args[0])
		s.Store.Save()
		fmt.Fprintf(w, "  %d invite code(s) cancelled\n", n)
	case "kick":
		if !need(1, "kick <user>") {
			return
		}
		fmt.Fprintln(w, " ", okText(s.Lobby.Kick(args[0], "You were kicked by the house.")))
	case "say":
		s.Lobby.Announce("[House] " + strings.Join(args, " "))
	case "admin":
		if !need(1, "admin <user> [on|off]") {
			return
		}
		on := len(args) < 2 || strings.EqualFold(args[1], "on") || strings.EqualFold(args[1], "yes") || args[1] == "1"
		if !s.Store.SetAdmin(args[0], on) {
			fmt.Fprintln(w, "  no such account")
			return
		}
		if on {
			fmt.Fprintf(w, "  %s is now an admin\n", args[0])
			s.Lobby.NotifyUser(args[0], "You're an admin now: type /help in chat for server commands.")
		} else {
			fmt.Fprintf(w, "  %s is no longer an admin\n", args[0])
		}
		s.Lobby.RefreshProfile(args[0])
	case "admins":
		if a := s.Store.Admins(); len(a) > 0 {
			fmt.Fprintln(w, "  admins:", strings.Join(a, ", "))
		} else {
			fmt.Fprintln(w, "  no admins yet ('admin <user>' makes one)")
		}
	case "get", "settings":
		filter := strings.ToLower(strings.Join(args, " "))
		for _, st := range SettingsOf(s.cfg()) {
			if filter != "" && !strings.Contains(st.Key, filter) {
				continue
			}
			v := st.Value
			if secretKeys[st.Key] && v != "" {
				v = "(set)"
			}
			fmt.Fprintf(w, "  %s = %s\n", st.Key, v)
		}
	case "set":
		if !need(2, "set <setting> <value>   ('get' lists them)") {
			return
		}
		if err := s.SetSetting(args[0], strings.Join(args[1:], " ")); err != nil {
			fmt.Fprintln(w, "  not changed:", err)
			return
		}
		s.Log.Printf("setting %s changed", args[0])
		fmt.Fprintf(w, "  %s changed; table settings apply at the next hand\n", args[0])
	case "tables":
		for _, l := range s.Lobby.TablesText() {
			fmt.Fprintln(w, l)
		}
	case "table":
		if !need(1, "table add|remove|rename|get|set ...  ('help' has the details)") {
			return
		}
		sub, rest := strings.ToLower(args[0]), args[1:]
		switch sub {
		case "add", "new", "open":
			if len(rest) < 2 {
				fmt.Fprintln(w, "usage: table add <blackjack|holdem> <name>")
				return
			}
			id, err := s.Lobby.AddTable(rest[0], strings.Join(rest[1:], " "))
			if err != nil {
				fmt.Fprintln(w, "  couldn't open it:", err)
				return
			}
			s.Log.Printf("opened table %s", id)
			fmt.Fprintf(w, "  opened %s\n", id)
		case "remove", "close", "delete":
			if len(rest) < 1 {
				fmt.Fprintln(w, "usage: table remove <id>")
				return
			}
			if err := s.Lobby.RemoveTable(rest[0]); err != nil {
				fmt.Fprintln(w, "  ", err)
				return
			}
			s.Log.Printf("closed table %s", rest[0])
			fmt.Fprintln(w, "  closed", rest[0])
		case "rename":
			if len(rest) < 2 {
				fmt.Fprintln(w, "usage: table rename <id> <name>")
				return
			}
			if err := s.Lobby.RenameTable(rest[0], strings.Join(rest[1:], " ")); err != nil {
				fmt.Fprintln(w, "  ", err)
				return
			}
			fmt.Fprintln(w, "  renamed")
		case "get", "settings":
			if len(rest) < 1 {
				fmt.Fprintln(w, "usage: table get <id>")
				return
			}
			list, err := s.Lobby.TableSettings(rest[0])
			if err != nil {
				fmt.Fprintln(w, "  ", err)
				return
			}
			for _, st := range list {
				fmt.Fprintf(w, "  %s = %s\n", st.Key, st.Value)
			}
		case "set":
			if len(rest) < 3 {
				fmt.Fprintln(w, "usage: table set <id> <setting> <value>")
				return
			}
			if err := s.Lobby.SetTableSetting(rest[0], rest[1], strings.Join(rest[2:], " ")); err != nil {
				fmt.Fprintln(w, "  not changed:", err)
				return
			}
			fmt.Fprintf(w, "  %s changed; it applies from the next hand\n", rest[1])
		default:
			fmt.Fprintln(w, "  table add|remove|rename|get|set")
		}
	case "update":
		s.Update(w, len(args) > 0 && strings.EqualFold(args[0], "force"))
	case "version":
		latest := s.Latest(false)
		fmt.Fprintf(w, "  server v%s; latest release v%s\n", Version, latest)
	case "reload":
		if err := s.reload(); err != nil {
			fmt.Fprintln(w, "  reload failed:", err)
		} else {
			fmt.Fprintln(w, "  config reloaded; table settings apply at the next hand")
		}
	case "save":
		if err := s.Store.Save(); err != nil {
			fmt.Fprintln(w, "  save failed:", err)
		} else {
			fmt.Fprintln(w, "  saved")
		}
	case "backup":
		if name, err := s.Store.Backup("manual", s.configPath); err != nil {
			fmt.Fprintln(w, "  backup failed:", err)
		} else {
			s.Store.PruneBackups(s.cfg().BackupKeep, s.cfg().BackupDays)
			fmt.Fprintln(w, "  saved", name)
		}
	case "backups":
		list := s.Store.Backups()
		if len(list) == 0 {
			fmt.Fprintln(w, "  no backups yet")
		}
		for i, n := range list {
			if i == 30 {
				fmt.Fprintf(w, "  ... and %d older\n", len(list)-30)
				break
			}
			fmt.Fprintln(w, " ", n)
		}
		fmt.Fprintln(w, "  folder:", s.Store.backupDir())
	case "restore":
		if !need(1, "restore <backup file name>   (see 'backups')") {
			return
		}
		if err := s.Store.Restore(args[0], s.configPath); err != nil {
			fmt.Fprintln(w, "  restore failed:", err)
			return
		}
		s.Log.Printf("restored backup %s; saving is paused until restart", args[0])
		fmt.Fprintln(w, "  restored. Saving is paused so nothing overwrites it: type 'quit' and start the server again to load it.")
		fmt.Fprintln(w, "  (the backup's config is in server.json.restored if you want it)")
	case "quit", "exit", "stop":
		s.quit()
	default:
		fmt.Fprintln(w, "  unknown command, try 'help'")
	}

}

func okText(ok bool) string {
	if ok {
		return "done"
	}
	return "no such account"
}

func signedChips(n int64) string {
	if n >= 0 {
		return "+" + commas(n)
	}
	return "-" + commas(-n)
}
