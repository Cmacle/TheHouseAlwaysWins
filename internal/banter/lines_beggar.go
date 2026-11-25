// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

// The Beggars Table: Dale running the soup kitchen. He's been broke more
// times than anyone (1984, 1996, the coffee can), and he hates seeing it in
// someone else almost as much as he enjoys it.

func init() {
	more(clean, EvBgHello,
		"Well, look who's at the Beggars Table. {name}. I've been where you are. Twice. Sit down.",
		"{name}. Broke already? Make the number and you're back in business. Don't hold your breath. Actually, do. It's free.",
		"Welcome to the bottom, {name}. The view's not great but the company's honest.",
		"{name} at the Beggars Table. My mother would've brought you a casserole. I'll bring you dice.",
		"Pull up a crate, {name}. In 1984 I slept in the Buick for a month. This is nicer. Barely.",
		"{name}, make the number and I'll give you {amount}. I'm not running a charity. I'm running exactly a charity.",
		"Down on your luck, {name}? Luck's a Tuesday thing. Today isn't Tuesday.",
	)
	more(vulgar, EvBgHello,
		"Well, shit. {name}'s broke. Welcome to the gutter, kid.",
		"{name} at the Beggars Table. Pathetic. I love it. Roll your dice, bum.",
		"Look at {name}, flat busted. I've seen hobos with better bankrolls. I've been the hobo.",
		"{name}. Make the number for {amount} chips. Good luck. You'll need a hell of a lot of it.",
		"Broke, {name}? Hell, I've been broke so many times I had business cards made.",
	)

	more(clean, EvBgRoll,
		"Again, {name}? Persistence. Or a problem.",
		"Roll 'em, {name}. Comp could do this all day. He has, actually.",
		"{name} keeps rolling. My grandfather rolled dice in a barbershop for a living. He died broke too.",
		"Come on, dice. Not for {name}'s sake. I just want to see it once.",
		"Another roll, {name}? Lorraine refills my coffee less often than you roll those.",
		"Those dice are older than you, {name}. They don't like you.",
		"Shake 'em, {name}. Blow on them. Pray. My mother would.",
	)
	more(vulgar, EvBgRoll,
		"Again? {name}, you're like a raccoon on a dumpster.",
		"Roll, beggar, roll.",
		"{name}, those dice hate you. I can feel it.",
		"Christ, {name}, how many more? I'm getting old here. Older.",
	)

	more(clean, EvBgMany,
		"{name} has rolled {streak} times. That's dedication. Misplaced, but dedication.",
		"{streak} rolls, {name}. Ray the Ferret would've given up and sold you a ferret by now.",
		"That's {streak} rolls for {name}. At this point I'm rooting for you. Don't tell anybody.",
		"{streak} rolls. {name}, I've had shorter marriages. All three of them.",
	)
	more(vulgar, EvBgMany,
		"{streak} rolls, {name}? Get a job. I'm kidding. I don't have one either. I have this.",
		"{name}'s at {streak} rolls. That's the saddest number I've ever heard, and I've seen my bank statement.",
	)

	more(clean, EvBgWin,
		"THERE IT IS! {name}! I don't believe it. Here's {amount}. Go. Before I change my mind.",
		"{name} did it! Forty-seven years and I've seen that maybe twice. Take your {amount} and get out of my soup kitchen.",
		"Well, I'll be. {name} made the number. My mother always said miracles happen. She meant church. But fine.",
		"WINNER! Somebody tell Lorraine! {name} is back in business with {amount}!",
		"{name} hits it. That's {amount} chips. Spend them wisely. You won't.",
	)
	more(vulgar, EvBgWin,
		"GODDAMN, THEY HIT! {name}, you lucky bastard! Take your {amount} and scram!",
		"Holy hell, {name} did it! Here's {amount}. Don't come crawling back. You'll come crawling back.",
		"Son of a bitch, they hit! {name}'s rich! Well, {amount} rich. Which is broke with a hat on.",
	)

	more(clean, EvBgTooRich,
		"{name}, you've got {amount} chips. This table's for people with nothing. Go lose those first.",
		"Nice try, {name}. {amount} chips isn't broke. Come back when you really mean it.",
		"{name}, the soup kitchen is for the hungry. You've got {amount}. You're snacking.",
	)
	more(vulgar, EvBgTooRich,
		"Beat it, {name}. You've got {amount} chips. Real bums only.",
		"{amount} chips and you're begging, {name}? That's low. Even for me.",
	)
}
