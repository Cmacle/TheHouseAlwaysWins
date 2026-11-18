// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

// Texas Hold'em lines. Forty-seven years dealing blackjack and Dale finally
// gets to play: he learned poker from his grandfather Tuck behind the
// barbershop, won Comp at Ray the Ferret's game, and cleaned out a
// submarine in 1971. All of it fits docs/DALE.md.

func init() {
	more(clean, EvPkHello,
		"{name} wants to play poker with me. Forty-seven years I dealt cards. Now I get to take your money personally.",
		"Sit down, {name}. Grandpa Tuck taught me this game behind a barbershop. With a ruler. You'll see why.",
		"Ah, {name}. Hold'em. Where I don't have to follow house rules. I just have to beat you.",
		"Welcome to the poker table, {name}. The cards are fair. I'm not.",
		"{name}, I won an iguana at a poker table in Shreveport. Let's see what I win off you.",
		"Pull up a chair, {name}. In blackjack I'm the house. Here I'm just a guy who's better than you.",
		"{name}. Good. I was getting bored playing against nobody. Nobody plays tighter than you, though.",
		"Hold'em with {name}. Lorraine at the Denny's said I'd find a fish today. She's never wrong.",
		"{name} at my poker table. Commander Hollis sat down like that in 1971. He left without his watch.",
		"Deal yourself in, {name}. No, wait, I deal. I always deal. That's the advantage.",
	)
	more(vulgar, EvPkHello,
		"{name}'s here for poker. Grab a seat, sucker. I mean friend. I mean sucker.",
		"Hold'em with {name}. I'm gonna take your money and you're gonna thank me for the lesson.",
		"Sit your ass down, {name}. Grandpa Tuck died at a card table. I plan to keep up the tradition by killing you at one.",
		"{name}, I won an iguana off a man named Ferret playing this game. You're not even a ferret.",
		"Look who wants to play cards with the big dog. {name}. Bring your wallet and your tissues.",
		"Poker with {name}. Hand me a Jim Beam, this is gonna be easy money.",
	)

	more(clean, EvPkSolo,
		"Just you and me, {name}. Heads up. Like a duel. Except I have more bullets.",
		"Heads up with {name}. Nowhere to hide now. Not for you, anyway.",
		"One on one, {name}. My father never played me heads up. Said I wasn't ready. I'm ready.",
		"Just the two of us, {name}. Every hand is a fight now. I like fights. I've been stabbed with a fork.",
		"{name}, heads up. You'll be playing every hand. Badly. But every hand.",
		"Nobody else here, {name}. No witnesses. That's how I play my best.",
		"Heads up poker, {name}. The purest form of the game. And the fastest way to lose.",
	)
	more(vulgar, EvPkSolo,
		"Just you and me, {name}. Mano a mano. Mano a dumbass.",
		"Heads up, {name}. Nobody to save you. Nobody to blame but your own damn self.",
		"One on one with {name}. I've arm-wrestled for a car. I can handle you.",
		"Two of us, {name}. Let's dance. I lead. You bleed.",
		"Heads up. {name}, I'm gonna run you over like a deer on Route 61.",
	)

	more(clean, EvPkLeave,
		"{name} leaves with {amount}. Run while you still have something to run with.",
		"{name} is cashing out. {amount}. Smart. First smart thing you've done all night.",
		"There goes {name} with {amount}. I'll get it next time. I always get it next time.",
		"{name}'s done. {amount} back to the bank. The bank thanks you. I don't.",
		"Leaving already, {name}? My second wife left like that. She took the bass boat.",
		"{name} walks with {amount}. Enjoy it. It's only borrowed.",
	)
	more(vulgar, EvPkLeave,
		"{name}'s running off with {amount}. Coward. Rich coward, but coward.",
		"Bye, {name}. Take your {amount} and your shame.",
		"{name} bails with {amount}. That's my money in your pocket. Remember that at night.",
		"There goes {name}. Like a rat off the Lucky Lady. At least the rat stayed for the sinking.",
	)

	more(clean, EvPkFold,
		"{name} folds. Wise. Your cards were sad. I could hear them crying.",
		"Fold, {name}? After putting in {amount}? You paid to watch. Like a movie.",
		"{name} gives up. Grandpa Tuck folded once in 1958. They talked about it for years.",
		"Folding again, {name}. You fold more than a laundromat.",
		"{name} lets it go. Probably the right call. Don't get used to being right.",
		"And {name} is out. {amount} in the pot that's not coming back to you.",
		"{name} folds. My mother folded laundry with more conviction.",
		"There it goes, {name}. Into the muck. Like my retirement plans.",
		"{name}'s out. Thank you for the donation. It's tax deductible. It isn't.",
		"{name} folds. Comp folds his legs under him when he's scared too. You two would get along.",
	)
	more(vulgar, EvPkFold,
		"{name} folds like a cheap lawn chair.",
		"Fold, {name}. Paid {amount} and quit. That's how my marriages went.",
		"{name} bitches out. Into the muck you go.",
		"There goes {name}, tail between the legs. Thanks for the {amount}, chickenshit.",
		"{name} folds. Squeal, little piggy.",
		"Folding? {name}, you got the spine of a jellyfish in a sauna.",
	)

	more(clean, EvPkCheck,
		"{name} checks. Oh, the suspense.",
		"Check from {name}. Bold strategy, doing nothing.",
		"{name} taps the table. That's what passes for courage around here.",
		"A check, {name}? You're scared. I can smell it. Or that's Comp.",
		"{name} checks. My father checked on me like that. Once.",
	)
	more(vulgar, EvPkCheck,
		"{name} checks. Weak. Weak as Denny's coffee.",
		"Check, {name}? Grow a pair. Or get dealt one.",
		"{name} knocks the table. Who's there? Nobody brave.",
	)

	more(clean, EvPkCall,
		"{name} calls {amount}. Curious, are we? Curiosity killed the cat. And the bankroll.",
		"{name} pays {amount} to see more. I hope you like what you see. You won't.",
		"A call from {name}. {amount}. Brave or stupid, we'll know in a minute.",
		"{name} sticks around for {amount}. Like a houseguest. Like my sister after the funeral.",
		"{name} calls. Fine. More for the pot. More for me.",
		"{amount} to stay in and {name} pays it. That's the spirit. The spirit of losing.",
	)
	more(vulgar, EvPkCall,
		"{name} calls {amount}. You don't fold and you don't raise. You just bleed.",
		"Calling station {name}. Beep beep, another {amount} on the train to my pocket.",
		"{name} calls. Can't let go, can you? Like Mitch with my kidney.",
		"{amount}, called. {name}'s got hope. Hope is for suckers.",
	)

	more(clean, EvPkRaise,
		"{name} makes it {amount}. Look at you. Big man. Big, temporary man.",
		"Raise to {amount}, {name}? Somebody's feeling lucky. Lucky is not a strategy.",
		"{name} bets {amount}. Either you've got something, or you watched a show about poker.",
		"{amount} from {name}. I've seen that bet before. In Reno. Guy lost his truck.",
		"Oh, {name} wants to play. {amount}. Let's see what you're made of. It's probably cheese.",
		"{name} puts {amount} out there. My grandfather would've called you a peacock. Then taken your feathers.",
		"Raising, {name}? With those hands? I mean the ones holding the cards. They're shaking.",
		"{name} bets {amount}. I'll allow it. For now.",
	)
	more(vulgar, EvPkRaise,
		"{name} raises to {amount}. Ooh, the big swinging wallet.",
		"{amount}, {name}? You're bluffing. Your nostrils do a thing. Like a horse.",
		"Raise to {amount}. {name} thinks they're a shark. You're a goldfish with a mortgage.",
		"{name} puts {amount} in. Bold move for somebody holding garbage.",
		"Look at {name}, betting {amount} like they've got a pair. You've got a prayer.",
	)

	more(clean, EvPkAllIn,
		"{name} is ALL IN. {amount}! This is it. This is the moment your life flashes before your eyes.",
		"All in, {name}? {amount}? I respect it. I don't respect you. But I respect it.",
		"{name} shoves {amount}! My father went all in once. He married my mother. Worked out, mostly.",
		"Everything in the middle. {name}, either you've got the goods or you've lost your mind.",
		"ALL IN from {name}! {amount}! Somebody call Lorraine, she'll want to hear about this.",
		"{name} pushes it all. {amount}. That's either genius or Tuesday. I never lose on a Tuesday.",
		"{name} moves all in. In 1971 a sailor did that on a submarine. He's still in the Navy. Working off the debt.",
	)
	more(vulgar, EvPkAllIn,
		"{name} SHOVES! {amount}! Holy hell, somebody's got balls or a death wish.",
		"All in, {name}? {amount}? You crazy son of a gun. I love it.",
		"{name} jams it all in. That's either the nuts or you're nuts.",
		"ALL IN! {name}, if you're bluffing, I'm gonna frame your cards and hang them in the trailer.",
		"{name} goes all in for {amount}. That's my kind of stupid.",
	)

	more(clean, EvPkWin,
		"{name} takes it. {amount}. Enjoy it. The universe is just setting you up.",
		"{name} wins {amount}. Even a broken clock. Even a broken clock.",
		"There's {amount} for {name}. Don't spend it all at once. Spend it all here.",
		"Pot goes to {name}. {amount}. I'm happy for you. That's a lie, I'm a liar, it's in my file.",
		"{name} drags {amount}. My father never let me win a hand. I'm not letting you win many.",
		"{name} wins with {item}. Fine. Fine! It's fine.",
		"{amount} to {name}. Lucky. Not good. Lucky. There's a difference and I'll explain it later.",
		"{name} scoops it. {amount}. Somewhere, Gene Petrakis is smiling. I hate that.",
	)
	more(vulgar, EvPkWin,
		"{name} takes the pot. {amount}. Lucky bastard.",
		"{amount} to {name}. Blind squirrel, meet nut.",
		"{name} wins. Hell. Fine. Take your {amount}. It's coming back to me anyway.",
		"{name} scoops {amount} with {item}. Even a hog finds a truffle.",
		"Pot to {name}. Don't get cocky. Cocky is my thing.",
	)

	more(clean, EvPkBigPot,
		"{name} takes down a monster! {amount}! That's more than my Buick is worth. Including the Buick.",
		"{amount} to {name}! I need to sit down. I am sitting down. I need to sit down more.",
		"What a pot! {name} wins {amount}. Ray the Ferret would've had you followed home.",
		"{name} rakes in {amount}! Somebody tell the Management it wasn't my fault.",
		"{amount}, {name}. That's a pot. That's a real pot. Grandpa Tuck died over less.",
	)
	more(vulgar, EvPkBigPot,
		"{name} rakes {amount}! Son of a bitch, that's a payday.",
		"{amount}! {name} just bought themselves a new truck. Or my old one.",
		"Holy hell, {name}. {amount}. I'm gonna need a stiffer drink.",
		"{name} cleans up {amount}. The Management's gonna call me. I'm not picking up.",
	)

	more(clean, EvPkMonster,
		"{name} shows {item}! Well, butter my biscuit. That's a hand.",
		"{item}! {name}, you don't see that every day. You see it once and then you lose it all chasing it again.",
		"{name} turns over {item}. I won Comp with a full house. You're not getting my iguana.",
		"{item} for {name}. My grandfather saw one of those and had a heart attack. He had several heart attacks. But one of them was that.",
		"Look at that. {name} with {item}. Take a picture. My granddaughter says that's what people do.",
	)
	more(vulgar, EvPkMonster,
		"{item}?! {name}, you lucky son of a bitch.",
		"{name} flips {item}. Well, shit. That's a hand.",
		"{item} for {name}. I've seen that hand once in forty years. Some drunk in Biloxi. He's dead now. Not related. Probably.",
		"Damn, {name}. {item}. Who'd you have to bribe?",
	)

	more(clean, EvPkRoyal,
		"A ROYAL FLUSH. {name}. A royal flush. I've dealt cards for forty-seven years. I need a minute.",
		"{name} has a ROYAL FLUSH! Somebody call my mother. She's dead. Call her anyway.",
		"Royal flush for {name}. My grandfather never saw one. My father never saw one. And I had to see it from you.",
		"ROYAL FLUSH! {name}, that's once in a lifetime. And it was your lifetime. Not mine. Never mine.",
	)
	more(vulgar, EvPkRoyal,
		"A ROYAL FLUSH?! {name}, you magnificent bastard. I'm gonna be sick.",
		"ROYAL. FLUSH. {name}. I've waited my whole damn life for that and you get it on a Wednesday.",
		"{name} hits a royal. Pour me a Beam. Pour me two. Pour me the bottle.",
	)

	more(clean, EvPkBadBeat,
		"Ooh, {name}. That's a bad beat. You had it. You had it! And then you didn't. Like my first marriage.",
		"So close, {name}. Your hand was good. Mine was better. Life's not fair. I'm proof.",
		"{name}, that hurt. I felt it. Then I counted my chips and felt better.",
		"Bad beat, {name}. I'd tell you I'm sorry but my mother taught me not to lie on Sundays. It's not Sunday.",
		"Tough one, {name}. {item} got you. Some days the river giveth. Today it gave to me.",
		"{name}, that was a good hand. Remember it fondly. It's gone now.",
	)
	more(vulgar, EvPkBadBeat,
		"OHHH, {name}! Brutal! You had me and I got there! Suck it!",
		"Bad beat, {name}. Cry about it. Cry into your empty wallet.",
		"{item} beats you, {name}. The river's a bitch and so am I.",
		"Sorry, {name}. That's not true. I'm the opposite of sorry.",
	)

	more(clean, EvPkBust,
		"{name} is out of chips. Rebuy or go home. Those are the options. I recommend rebuy.",
		"And {name} is busted. Don't worry. Everybody busts eventually. I busted in 1984 and again in 1996.",
		"{name}'s stack is gone. Like my coffee can. No, the coffee can's fine. Don't look for it.",
		"Busted, {name}. Grandpa Tuck would say 'never let them see you care.' Too late, I can see it.",
		"{name} is felted. That's poker talk. It means you lost everything. I learned it from a podcast. Not DJ's.",
	)
	more(vulgar, EvPkBust,
		"{name}'s busted flat. Get your ass to the cash machine.",
		"Felted, {name}! Out of chips, out of luck, out of my sight.",
		"{name} is broke. Rebuy, you coward. The table misses your money.",
		"And {name} is dead. Rest in peace. Rest in pieces.",
	)

	more(clean, EvPkRebuy,
		"{name} buys back in for {amount}. That's commitment. Or a problem. Possibly both.",
		"Another {amount}, {name}? I admire a person who doesn't learn.",
		"{name} reloads with {amount}. Welcome back. Your old chips say hi. They're over here now.",
		"Rebuying, {name}? {amount}? Like a man going back to Rhonda. Don't do it. Do it.",
	)
	more(vulgar, EvPkRebuy,
		"{name} reloads with {amount}. Fresh meat for the grinder.",
		"Back for more, {name}? {amount}? You glutton for punishment.",
		"{amount} more from {name}. Like feeding quarters to a slot machine with a mustache.",
	)

	more(clean, EvPkSlow,
		"Any day now, {name}. I'm seventy-five. I don't have that many days.",
		"{name}, it's not chess. Well, it's a little bit chess. But faster.",
		"Take your time, {name}. I'll just sit here and age.",
		"{name} is thinking. That's new. Let's give them a minute.",
		"Clock's ticking, {name}. Lou wants his money Friday. I'd like to play a few hands before then.",
	)
	more(vulgar, EvPkSlow,
		"Christ, {name}, make a decision before I die. I'm close.",
		"{name}, I've seen glaciers move faster. Fold or bet, damn it.",
		"Hurry up, {name}. My cigar's going out and so's my patience.",
	)

	more(clean, EvPkFlop,
		"Ooh, look at that flop. Somebody just got very happy. Or very scared.",
		"That's a scary board. My second wife looked at me like that.",
		"Interesting flop. Grandpa Tuck would've had something to say. Probably a swear. He was a Baptist, but at cards he swore.",
		"Look at that. That flop changes things. Not for me. I'm always fine.",
	)
	more(vulgar, EvPkFlop,
		"Holy hell, look at that flop. Somebody's about to get screwed.",
		"That's an ugly board. Ugly like my ex-wife's brother. The fork one.",
		"Whoo, that flop. Somebody just wet themselves.",
	)

	// ------------------------------------------------------------ Dale himself

	more(clean, EvPkDaleRaise,
		"I'll make it a little more interesting.",
		"Raise. Because I can.",
		"Let's see if anybody here has a spine.",
		"A little bet. Just to see who's awake.",
		"I raise. My father would've. Well, he'd have folded. I'm not my father.",
		"Bet. Forty-seven years I wanted to say that from this side of the table.",
		"I'm putting some chips in. Don't read into it. Read into it.",
		"Raise. Comp told me to. He's never wrong.",
	)
	more(vulgar, EvPkDaleRaise,
		"Raise. Put up or shut up.",
		"I'm betting. Deal with it, you bunch of mopes.",
		"Here's some chips. Choke on them.",
		"Raise. Come at me.",
		"Bet. Your move, chickenshits.",
	)

	more(clean, EvPkDaleCall,
		"I'll call. Let's see where this goes.",
		"Call. I'm curious. That's dangerous for you.",
		"I'll pay to see. I've paid more to see less. Branson, mostly.",
		"Call. Don't get excited.",
	)
	more(vulgar, EvPkDaleCall,
		"Fine. I call. Show me what you got, which is nothing.",
		"Call. I smell a bluff.",
		"I'll see your stupid bet.",
	)

	more(clean, EvPkDaleCheck,
		"Check.",
		"I'll check. For now.",
		"Check. I'm being polite. It won't last.",
	)
	more(vulgar, EvPkDaleCheck,
		"Check. Don't get comfortable.",
		"Check. Go ahead, hang yourself.",
	)

	more(clean, EvPkDaleFold,
		"I fold. Strategically. It's a strategy.",
		"Fold. Don't tell the Management.",
		"I'll let that one go. I'm letting you win. That's a lie but go with it.",
		"Fold. My grandfather said pick your battles. He picked badly. I pick well.",
		"I'm out. Enjoy it. It won't happen often.",
	)
	more(vulgar, EvPkDaleFold,
		"Fold. Screw it. Take it.",
		"I fold. Don't get used to it, punks.",
		"Bah. Fold. Garbage cards. Garbage dealer. Wait, I'm the dealer.",
	)

	more(clean, EvPkDaleAllIn,
		"All in. Every chip. Like Grandpa Tuck. Hopefully with a better ending.",
		"I'm all in. Let's find out who you really are.",
		"Everything in the middle. Your move. Choose wisely. You won't.",
		"All in. I haven't felt this alive since the Lucky Lady ran aground.",
		"All of it. Call me if you dare. Nobody ever dares.",
	)
	more(vulgar, EvPkDaleAllIn,
		"ALL IN, baby. Call or cry.",
		"Everything. Shoved. Put your money where your mouth is.",
		"All in. Go ahead, call. I dare you. I double dog dare you.",
		"I'm shoving. Hold onto your ass.",
	)

	more(clean, EvPkDaleWin,
		"That one's mine. {amount}. Thank you, {name}. Thank you very much.",
		"The house takes it. Even at poker, the house takes it.",
		"Mine. {amount}. Comp's getting collard greens tonight.",
		"I'll take that, {name}. Grandpa Tuck would be proud. Somebody should be.",
		"{amount} to the dealer. That's me. I'm the dealer.",
		"I win. I'd say good game, {name}, but I don't lie on Tuesdays. Or I do. I forget which.",
		"Pot's mine. Write it down. Not you. The house.",
	)
	more(vulgar, EvPkDaleWin,
		"Mine, bitches. {amount}.",
		"Read 'em and weep, {name}. Mostly weep.",
		"{amount} to the old man. Who's laughing now? Me. I'm laughing. Wheeze.",
		"That's how it's done, {name}. Take notes. You won't, you'll just keep losing.",
		"Ka-ching. Thanks, {name}. That's next month's lot rent.",
	)

	more(clean, EvPkDaleLose,
		"{item}. {name} beats me for {amount}. I'm going to need a moment. And a Folgers.",
		"Fine, {name}. {amount}. Fine. I'll remember this. I have a long memory. I remember 1983.",
		"{name} wins with {item}. My father lost to people like you. Now I know how he felt. It's bad.",
		"Well. {name}. {item}. Don't tell the Management. Don't tell Comp.",
		"That's {amount} out of my cut, {name}. I hope you're happy. You look happy. Stop it.",
		"{name} beats me. Ray the Ferret beat me once. Now I owe him eleven grand. You're not getting eleven grand.",
	)
	more(vulgar, EvPkDaleLose,
		"Son of a bitch. {item}. {name} takes {amount} off me.",
		"Damn it, {name}! {amount}! That's my cigar money!",
		"{item}. Unbelievable. {name}, you got lucky and you know it.",
		"Hell. {name} beats me. Somebody pour me a drink before I flip this table.",
		"{amount} gone to {name}. The Management is gonna ream me.",
	)

	more(clean, EvPkDaleBluff,
		"I had nothing. Absolutely nothing. Look. Seven high. Grandpa Tuck taught me that one.",
		"You folded to this? I had air. Commander Hollis folded to air in 1971 too.",
		"Bluff. Total bluff. Forty-seven years of watching people prepared me for exactly that.",
		"Here, I'll show you. Nothing. I had nothing and you believed me. Everybody believes me. That's my curse.",
		"I'm showing you because I'm a generous man. That was a bluff. You can cry now.",
	)
	more(vulgar, EvPkDaleBluff,
		"BLUFF! I had jack squat and you ran like a scared rabbit!",
		"Look at this garbage. That's what you folded to. Hahaha. Pathetic.",
		"I had nothing. NOTHING. You got outplayed by a seventy-five-year-old man in a trailer.",
		"Bluffed your ass. Let that sink in. Let it sink like the Lucky Lady.",
	)
}
