// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

// Baccarat and Red Dog. Dale learned baccarat dealing to high rollers on the
// Lucky Lady riverboat (he hated every tuxedo of it) and Red Dog from the
// back room at Ray the Ferret's, where it was called Acey-Deucey and you
// didn't ask questions.

func init() {
	// ---------------------------------------------------------- baccarat
	more(clean, EvBacHello,
		"{name} at the baccarat table. Fancy. I dealt this on the Lucky Lady to men in tuxedos. None of them tipped.",
		"Baccarat, {name}? It's the game for people who want to gamble but don't want to think. Welcome.",
		"Sit down, {name}. Player, Banker or Tie. Even you can't get that wrong. Well. You can. Two out of three times.",
		"{name}. Baccarat is what James... a certain spy plays. You are not him. You have the posture of a folding chair.",
		"Welcome to baccarat, {name}. Lorraine says it's pronounced 'bah-cah-rah'. Lorraine has never been on a boat.",
	)
	more(vulgar, EvBacHello,
		"{name}'s playing baccarat. Look at Mister Monte Carlo over here. You're in a trailer, pal.",
		"Baccarat, {name}? The game for rich jerks. Except you're broke. So just a jerk.",
		"Sit your fancy ass down, {name}. Player, Banker, Tie. Pick one and pray.",
	)
	more(clean, EvBacDeal,
		"No more bets. Here come the cards.",
		"Bets are down. Player, then Banker.",
		"That's it, hands off the felt. Let's see some cards.",
		"No more bets, ladies and gentlemen. And whoever else is here.",
		"Cards are coming out. Hold your breath. Not literally. Last time someone fainted.",
	)
	more(vulgar, EvBacDeal,
		"No more bets. Hands off the goddamn felt.",
		"Bets are in. Let's see who's crying in a minute.",
		"Cards coming. Pucker up.",
	)
	more(clean, EvBacNatural,
		"A natural {amount} for the {item}! That's the hand. Nobody draws.",
		"{item} turns over a natural {amount}. Game over, man.",
		"Natural {amount}! The {item} doesn't even break a sweat.",
		"{amount} in two cards for the {item}. The tuxedos used to clap for that. Politely. With gloves on.",
	)
	more(vulgar, EvBacNatural,
		"Natural {amount} for the {item}. Bam. Done. Suck it.",
		"{item} flips a natural {amount}. Somebody's eating good tonight. Somebody isn't.",
	)
	more(clean, EvBacPlayer,
		"Player wins.",
		"Player takes it. Player bettors, collect. Banker bettors, reflect.",
		"The Player hand wins. Mark it down.",
		"Player. Again. Or for the first time. I lose track. I'm seventy-five.",
		"Player wins it. Ray the Ferret always bet Player. He's in prison. Unrelated.",
	)
	more(vulgar, EvBacPlayer,
		"Player wins. Banker bettors, eat it.",
		"Player takes it. Pay the lucky bastards.",
	)
	more(clean, EvBacBanker,
		"Banker wins. The house gets its little five percent. Or it would, if we charged one.",
		"Banker takes it. The smart money's smiling.",
		"Banker wins. My father always bet the Banker. He also always lost. Different game.",
		"Banker hand wins. Collect it, Banker people.",
		"Banker. Like the ones who took the Buick.",
	)
	more(vulgar, EvBacBanker,
		"Banker wins. Player bettors, kiss it goodbye.",
		"Banker. Ha. Like the bastards who took my Buick.",
	)
	more(clean, EvBacTie,
		"A TIE! Eight to one on the tie bets! Player and Banker bets push.",
		"It's a tie! Somebody bet on that? Nobody bets on that. Somebody should bet on that.",
		"Tie hand. The tie bettors just got very rich and very annoying.",
		"A tie. I've seen it maybe once a night for forty-seven years. Never stops being weird.",
	)
	more(vulgar, EvBacTie,
		"A goddamn TIE! Eight to one! Who bet that, you psychic freak?",
		"Tie. Everybody pushes except the lunatics who bet the tie. They get paid. Lunatics.",
	)
	more(clean, EvBacBigWin,
		"{name} wins {amount}. Don't let it go to your head. It will. But try.",
		"{amount} to {name}. The tuxedo crowd would've nodded. Slightly.",
		"{name} cleans up, {amount}. Even the Lucky Lady never paid like that. She sank.",
	)
	more(vulgar, EvBacBigWin,
		"{name} rakes in {amount}. Lucky son of a gun.",
		"{amount} for {name}. The Management's gonna have my hide.",
	)
	more(clean, EvBacBigLose,
		"{name} drops {amount}. Baccarat giveth. Mostly taketh.",
		"Ooh. {amount} gone, {name}. You picked a side. The wrong side. But a side.",
		"{name} loses {amount}. My condolences. My insincere condolences.",
	)
	more(vulgar, EvBacBigLose,
		"{name} just blew {amount}. Nice going, Rockefeller.",
		"{amount} down the drain, {name}. Should've bet the other one, dummy.",
	)
	more(clean, EvBacStreak,
		"{item} has won {amount} in a row. Somebody's going to tell you it's due to change. They're wrong. Cards don't remember.",
		"That's {amount} straight for the {item}. Ride it or fight it. I don't care. I get paid either way. I don't, actually.",
		"{amount} {item} wins in a row. The tuxedos would be sweating through their cummerbunds.",
	)
	more(vulgar, EvBacStreak,
		"{amount} in a row for the {item}. Somebody's gonna lose their shirt betting against that.",
		"{item}, {amount} straight. The streak's hot. Like my trailer in July.",
	)

	// ---------------------------------------------------------- Red Dog
	more(clean, EvRdHello,
		"{name} wants to play Red Dog. We called it Acey-Deucey at Ray the Ferret's. You didn't ask why.",
		"Red Dog, {name}. Two cards, and you bet the third lands between them. Simple. Like you.",
		"Sit down, {name}. Red Dog. The game with the dumbest name and the best back room.",
		"{name} at Red Dog. Comp likes this game. He doesn't play. He watches. Judgmentally.",
	)
	more(vulgar, EvRdHello,
		"{name}'s at Red Dog. Woof. Let's see if you're a good boy or a dead dog.",
		"Red Dog, {name}? Grab a seat, mutt.",
		"{name}. Red Dog. Ray the Ferret ran this game out of a meat locker. You'd have fit right in.",
	)
	more(clean, EvRdSpread,
		"Spread of {amount}. That pays {item}. Raise if you're feeling brave.",
		"{amount} cards between them. Pays {item}. Want to double down? Not that kind. This kind.",
		"A {amount}-card spread. {item} if it hits. Decide quick, I'm not getting younger.",
		"Spread's {amount}, paying {item}. Grandpa Tuck would raise. Grandpa Tuck died broke.",
	)
	more(vulgar, EvRdSpread,
		"Spread of {amount}, pays {item}. Raise or shut up.",
		"{amount}-card gap. {item}. Grow a pair and double it.",
	)
	more(clean, EvRdConsec,
		"Consecutive. Nothing fits between those. Everybody pushes.",
		"Back to back cards. No room in between. Push.",
		"Consecutive cards. It's a push. Exciting, I know.",
	)
	more(vulgar, EvRdConsec,
		"Consecutive. Push. Boring as hell.",
		"No gap. Push. Wake me up when something happens.",
	)
	more(clean, EvRdPair,
		"A pair! One more card. Hit three of a kind and it pays eleven to one.",
		"Pair on the table. Here comes the third. Pray for its twin.",
		"Matching cards. If the next one matches too, I start sweating.",
	)
	more(vulgar, EvRdPair,
		"A pair! One more, and if it matches I'm paying eleven to one. Hell.",
		"Pair. Come on, triplets. No, don't come on. Damn it.",
	)
	more(clean, EvRdTrips,
		"THREE OF A KIND! Eleven to one! Everybody gets paid!",
		"Trips! I haven't seen that since the meat locker. Eleven to one!",
		"Three of a kind. Comp just stood up. Comp never stands up. Eleven to one!",
	)
	more(vulgar, EvRdTrips,
		"THREE OF A KIND! Son of a bitch! Eleven to one, you lucky dogs!",
		"Trips! Holy hell! The Management's gonna kill me!",
	)
	more(clean, EvRdRaise,
		"{name} doubles up. Bold. Stupid, maybe. But bold.",
		"{name} raises. I like the confidence. I don't like the odds.",
		"Double the bet, {name}? Ray the Ferret would've loved you. Briefly.",
	)
	more(vulgar, EvRdRaise,
		"{name} raises. Big balls, small brain.",
		"{name} doubles down. Somebody's feeling like a big shot.",
	)
	more(clean, EvRdWin,
		"{name} wins {amount}. Right between the posts.",
		"In between! {name} collects {amount}.",
		"{amount} to {name}. The card went right where it was told. Unlike my children.",
	)
	more(vulgar, EvRdWin,
		"{name} threads the needle for {amount}. Lucky mutt.",
		"In between, damn it. {amount} to {name}.",
	)
	more(clean, EvRdMiss,
		"Outside the spread. The house takes it.",
		"Missed. That card didn't want to be between anything. I relate.",
		"Outside. Thank you, everybody. The house appreciates your support.",
		"No good. Comp, write that down.",
	)
	more(vulgar, EvRdMiss,
		"Missed! Ha! Pay up, dogs.",
		"Outside the spread. Thanks for the chips, suckers.",
	)
	more(clean, EvRdGutshot,
		"A one-card spread and it HIT! Five to one! I don't believe it.",
		"Gutshot! The only card that works, and there it is. Five to one.",
	)
	more(vulgar, EvRdGutshot,
		"A one-card spread and it hit?! Five to one! Bullshit! Fine. Pay 'em.",
		"Gutshot, you magnificent bastards. Five to one.",
	)
}
