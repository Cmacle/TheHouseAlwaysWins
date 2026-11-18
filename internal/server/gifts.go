package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Chip gifts let you buy a broke friend back in without letting anyone pump
// one account: a gift can only bring someone up to the refill amount
// (counting chips they have on the table), there's a daily cap on what one
// player can receive, and gifts never count as winnings.

// staked is what a player has bet but not yet got back.
func (g *Game) staked(user string) int64 {
	p := g.byUser[strings.ToLower(user)]
	if p == nil {
		return 0
	}
	var n int64
	if p.betPlaced && !p.inHand {
		n += p.pendingBet
	}
	if p.inHand {
		for _, h := range p.hands {
			if h.Result == "" {
				n += h.Bet
			}
		}
	}
	return n
}

// giftMax is the most from can send to now, or 0 and the reason.
func (g *Game) giftMax(from *player, to string) (int64, string) {
	refill := g.cfg.RefillAmount
	prof, ok := g.store.Profile(to)
	switch {
	case !g.cfg.ChipGifts:
		return 0, "Chip gifts are turned off on this server."
	case !ok:
		return 0, "There's no player called " + to + "."
	case strings.EqualFold(prof.Username, from.user):
		return 0, "You can't send chips to yourself."
	case refill <= 0:
		return 0, "Gifts need a refill amount set on the server."
	}
	have := prof.Balance + g.staked(prof.Username)
	if have >= refill {
		return 0, fmt.Sprintf("%s has %s chips. You can only help someone with less than %s.", prof.Username, commas(have), commas(refill))
	}
	max, why := refill-have, fmt.Sprintf("Up to %s: that brings them to %s.", commas(refill-have), commas(refill))
	if g.cfg.GiftRefillsPerDay > 0 {
		left := int64(g.cfg.GiftRefillsPerDay*float64(refill)) - g.store.GiftedIn(prof.Username, g.cfg.RefillPeriod(time.Now()))
		if left <= 0 {
			return 0, prof.Username + " has had all the gifts they can get today. Try again after the refill."
		}
		if left < max {
			max, why = left, fmt.Sprintf("Up to %s: the rest of what %s can be sent today.", commas(left), prof.Username)
		}
	}
	mine := g.store.Balance(from.user)
	if from.betPlaced && !from.inHand {
		mine -= from.pendingBet
	}
	if mine <= 0 {
		return 0, "You don't have any chips to send."
	}
	if mine < max {
		max, why = mine, "Up to "+commas(mine)+": that's all you've got off the table."
	}
	return max, why
}

func (g *Game) onGift(from *player, to string, amount int64) {
	max, why := g.giftMax(from, to)
	if max <= 0 {
		g.errTo(from, why)
		return
	}
	if amount <= 0 {
		g.errTo(from, "Send at least 1 chip.")
		return
	}
	if amount > max {
		g.errTo(from, why)
		return
	}
	prof, _ := g.store.Profile(to)
	if err := g.store.Gift(from.user, prof.Username, amount, g.cfg.RefillPeriod(time.Now())); err != nil {
		g.errTo(from, err.Error())
		return
	}
	g.log.Printf("gift: %s sent %d chips to %s", from.user, amount, prof.Username)
	seat := -1
	if tp := g.byUser[strings.ToLower(prof.Username)]; tp != nil {
		seat = tp.seat
		g.sendProfile(tp)
	}
	g.sendProfile(from)
	g.eventAll(proto.Event{Kind: "gift", Seat: seat, Delta: amount,
		Text: fmt.Sprintf("%s sent %s %s chips", from.user, prof.Username, commas(amount))})
	if g.cfg.DealerTrashTalk {
		g.say(banter.EvGift, from, banter.Vars{Amount: amount, Item: prof.Username}, 0.9, prioNormal)
	}
	g.broadcast()
}
