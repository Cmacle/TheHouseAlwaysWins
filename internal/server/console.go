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
func (s *Server) RunConsole(r io.Reader, w io.Writer, reload func() error, quit func()) {
	sc := bufio.NewScanner(r)
	fmt.Fprintln(w, "Type 'help' for console commands.")
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
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
  kick <user>                 disconnect a player
  say <message>               message everyone at the table
  reload                      reload server.json (applies at the next hand; port changes need a restart)
  save                        write accounts to disk now
  quit                        save and shut down
`)
		case "players", "who":
			for _, l := range s.Game.PlayerList() {
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
				continue
			}
			p, ok := s.Store.Profile(args[0])
			if !ok {
				fmt.Fprintln(w, "  no such account")
				continue
			}
			fmt.Fprintf(w, "  %s  balance %d  net %+d  won %d  lost %d  refills %d\n  hands %d (W%d L%d P%d)  blackjacks %d  biggest win %d  online %v\n",
				p.Username, p.Balance, p.Net, p.Won, p.Lost, p.RefillTotal, p.HandsPlayed, p.HandsWon, p.HandsLost, p.HandsPushed, p.Blackjacks, p.BiggestWin, p.Online)
		case "give":
			if !need(2, "give <user> <amount>") {
				continue
			}
			amt, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				fmt.Fprintln(w, "  bad amount")
				continue
			}
			bal, ok := s.Store.AdminAdjust(args[0], amt)
			if !ok {
				fmt.Fprintln(w, "  no such account")
				continue
			}
			fmt.Fprintf(w, "  %s now has %d\n", args[0], bal)
			s.Game.NotifyUser(args[0], fmt.Sprintf("The house adjusted your chips by %+d.", amt))
			s.Game.notify.LeaderboardChanged()
		case "refill":
			if !need(1, "refill <user|all>") {
				continue
			}
			users := []string{args[0]}
			if strings.EqualFold(args[0], "all") {
				users = s.Store.Usernames()
			}
			var done []string
			for _, u := range users {
				if s.Game.InHand(u) {
					fmt.Fprintf(w, "  %s is mid-hand, skipped\n", u)
					continue
				}
				if d := s.Store.ForceRefill(u, s.cfg().RefillAmount); d > 0 {
					done = append(done, u)
					fmt.Fprintf(w, "  %s +%d\n", u, d)
					s.Game.NotifyUser(u, fmt.Sprintf("Refill: +%d chips", d))
				}
			}
			if len(done) > 0 {
				s.Game.notify.Refilled(done, s.cfg().RefillAmount)
				s.Game.notify.LeaderboardChanged()
			}
			s.Game.Broadcast()
		case "resetstats":
			if !need(1, "resetstats <user>") {
				continue
			}
			if s.Game.InHand(args[0]) {
				fmt.Fprintln(w, "  player is mid-hand, try again in a moment")
				continue
			}
			fmt.Fprintln(w, " ", okText(s.Store.ResetStats(args[0], s.cfg().RefillAmount)))
			s.Game.Broadcast()
			s.Game.notify.LeaderboardChanged()
		case "setpassword":
			if !need(2, "setpassword <user> <password>") {
				continue
			}
			fmt.Fprintln(w, " ", okText(s.Store.SetPassword(args[0], args[1])))
		case "delete":
			if !need(1, "delete <user>") {
				continue
			}
			s.Game.Kick(args[0], "Your account was deleted.")
			fmt.Fprintln(w, " ", okText(s.Store.Delete(args[0])))
			s.Game.notify.LeaderboardChanged()
		case "kick":
			if !need(1, "kick <user>") {
				continue
			}
			fmt.Fprintln(w, " ", okText(s.Game.Kick(args[0], "You were kicked by the house.")))
		case "say":
			s.Game.Announce("[House] " + strings.Join(args, " "))
		case "reload":
			if err := reload(); err != nil {
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
		case "quit", "exit", "stop":
			quit()
			return
		default:
			fmt.Fprintln(w, "  unknown command, try 'help'")
		}
	}
}

func okText(ok bool) string {
	if ok {
		return "done"
	}
	return "no such account"
}
