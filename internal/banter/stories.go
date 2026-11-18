// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

import "regexp"

// Dale's long stories, told a page at a time now and then. Each one fits the
// same life story (see docs/DALE.md). [clean|vulgar] picks the wording for
// each mode; {name} is a player at the table.

// Story events.
const (
	EvStory  = "story"  // a long multi-paragraph story
	EvResume = "resume" // getting back to a story after an interruption
)

var stories = []string{
	// the Lucky Lady, 1983
	`You want to know about the Lucky Lady? Fine. Summer of 1983. A "dinner cruise" out of Memphis. The dinner was a ham sandwich. The cruise was a back room with three blackjack tables, a roulette wheel with a lean, and me. The best dealer on the Mississippi River.

The captain was a man named Lyle Boudreaux. Lyle had one eye, two ex-wives, and a [thirst|drinking problem] that would kill a horse. That night the sheriff's boat came up the river with its lights on, and Lyle decided the Lucky Lady could outrun it. The Lucky Lady could not outrun a duck.

He put her straight onto a sandbar south of Tunica at full steam. The whole boat tipped like a drunk on a barstool. Chips everywhere. People screaming. A man in a white suit tried to climb out a porthole with the roulette wheel.

And what did I do? I closed my table. I racked every chip, I lifted that tray over my head, and I walked into the Mississippi River. Some people say the water only came up to my waist. Those people are liars. And also correct.

I walked a mile to shore with that tray. Didn't drop a chip. The owner never paid me for that night, so I kept the tray. It's in my trailer. Comp sleeps in it. And that, {name}, is the difference between you and me. When the ship goes down, I save the money. You'd have saved the ham sandwich.`,

	// Grandpa Tuck
	`My grandfather was Augustus Kroft. Everybody called him Tuck, because he could tuck an ace up his sleeve faster than you could blink. He ran a card game in the back of a barbershop in Cape Girardeau, Missouri. Nobody ever got a haircut.

I was born in that back room in 1951. On the card table. Grandpa Tuck said I came out face down and he had to flip me over to see what I was. He said I was a deuce. For the first thirty years, he wasn't wrong.

He taught me to shuffle when I was five. Riffle, bridge, cut. He'd smack my knuckles with a ruler every time I flashed a card. By seven I could deal a perfect round in the dark. By eight I had arthritis.

He died in 1962, sitting at that table, holding two pair. Jacks and sevens. Never got to see the last card. Nobody touched his chips for an hour, out of respect. Then they split them. Also out of respect.

His last words to me were, "Never let them see you care." I've lived by that every day of my life. So when you lose, {name}, and you look over here hoping I feel something? I don't. Grandpa Tuck made sure of that.`,

	// Earl, 1979
	`My father, Earl Kroft, dealt cards on the river for thirty-one years. Quiet man. Big hands. Never raised his voice, never told a joke, never once said he was proud of me. I know because I kept count. The count is zero.

In the spring of 1979 he got me a tryout on the Lucky Lady. I was twenty-eight. Kicked out of the Navy, run out of Reno for reasons we will not discuss, and dealing poker in a bowling alley. He watched me deal for an hour. Then he said, "Your shuffle's sloppy." That's the nicest thing he ever said to me.

Two weeks later, on a Tuesday, he dealt his last hand. Heart attack, right in the middle of a shuffle. He finished the shuffle. Dealt the round. Then he lay down on the floor behind the table, and that was that. Nobody noticed for ten minutes. He'd have liked that.

They gave me his seat the next night. His seat, his visor, his tray. That's where I've been ever since. Forty-seven years. And that's why nobody beats me on a Tuesday. I won't allow it. Not on his day.

Sometimes I think if I deal enough hands and take enough money off enough [fools|idiots], he'll finally look down and say it. "Good job, Dale." [Oh, stop|Christ, stop] looking at me like that, {name}. Bet your chips.`,

	// the Navy
	`1969. Everybody my age was getting drafted, and my mother said the Navy was safer. So I joined the Navy, and they put me on a submarine. A steel tube full of men who haven't seen the sun in two months. Safer.

There's nothing to do on a submarine. You can sleep, you can work, or you can watch a man named Kowalski eat sardines. So I made a blackjack table out of a mess tray and a wool blanket. Within a week I had the whole crew playing. Within a month I had the whole crew's paychecks.

The captain found out because the cook stopped cooking. He owed me so much he figured, what's the point. Commander Hollis came down to shut the game down, and instead he sat down. Hit sixteen against a six every single time. Man had the instincts of a lemming.

By the end of that night I owned his watch, his good pen, and his share of the Christmas ham. Two weeks later I owned a discharge. Not dishonorable. Just disappointed. The paper said "unsuitable temperament." I had it framed.

That's the thing about a submarine, {name}. There's no way out. You can't walk away from the table. You just sit there, two hundred feet down, losing your paycheck to a kid from Missouri. Which, now that I think about it, is exactly what you're doing right now.`,

	// the Velvet Kroft, Reno 1976
	`In 1976 I was a lounge singer at the Silver Spur in Reno. They called me The Velvet Kroft. Powder blue tuxedo, a mustache you could set a drink on, and a voice like a warm bath with a radio in it.

Two shows a night. Engelbert Humperdinck, Tom Jones, a little Neil Diamond when the crowd was drunk enough. My mother taught me piano, and it's the only thing she ever gave me that I didn't pawn. Women threw room keys. Men threw ashtrays. Both are a sign of respect.

Now, people ask me why it ended. I'm not allowed to tell you. I signed something. There was a lawyer, a priest, and a man from the Nevada Gaming Commission in that room, and all three of them were crying.

I will say this much. It involved the owner's wife, a rented llama, and the song "Sweet Caroline." I didn't know the llama was rented. I didn't know the owner had a wife. And I didn't know you could get banned from a song.

To this day, when that song comes on, I have to leave the building. Legally. It's in the paperwork. So if any of you ever hum it at this table, I'm taking your chips with me to the parking lot. [Rules are rules.|Rules are goddamn rules.]`,

	// Darlene and the mime
	`My first wife was Darlene. Cocktail waitress on the Lucky Lady. Hair so big it had its own weather. She could carry nine drinks and a grudge at the same time. We got married in 1980, in the boat's back room, on my table. My father would've hated it. That was half the appeal.

When the Lucky Lady sank, I was out of work for a year. Darlene got a job down on the Memphis riverfront, and that's where she met him. Pierre. A street mime. White face, striped shirt, the whole act. His real name was Gary. He was from Tulsa.

She left me in 1984. Left a note on the fridge: "Gone with Pierre." I went down to the riverfront to fight him. You ever try to fight a mime? He pretended to be in a box. I hit the box. The box hit back. I still don't know how.

The worst part? She said he was a better listener. He's a MIME, Darlene. He isn't listening. He just isn't TALKING. There's a difference. I've spent forty years explaining that difference to people in bars, and not one of them gets it.

Last I heard they live in Branson. Same town as my timeshare. Second week of February I go down there, sit by the pool, and watch for a man pulling an invisible rope. Haven't seen him yet. But I'm patient, {name}. The house is always patient.`,

	// Rhonda, the fork and the kidney
	`My second wife, Rhonda, was a pit boss at the Golden Catfish in Tunica. Toughest woman I ever met. She once threw a man out of the casino by his ear. He was six foot five. She was five foot one. She finished her cigarette while she did it.

We got married in 1990, and our boy came along in '91. For a few years there I had a house with a yard, a bass boat, a dog named Biscuit, and a wife who scared me. Best years of my life. I didn't notice at the time. You never do.

Rhonda had a brother named Mitch. Mitch was trouble. In 1992, Mitch stabbed me with a fork at a Sizzler because I said his steak was overcooked. It was overcooked. I finished my steak with the fork still in my arm. Out of principle.

Four years later, Mitch's kidneys quit. Doctors said family was the best match. Rhonda wasn't a match. Somehow, I was. So I gave the man who stabbed me with a fork my left kidney. And a month later, Rhonda divorced me anyway. That's why I tell people she took it in the divorce.

She got the house. She got the boat. She got Biscuit. Mitch got a kidney. I got a scar and a lifetime of eating at Sizzler with my back to the wall. So when I say the house always wins, {name}, understand that I have been the house, and I have lost the house.`,

	// Crystal, Biloxi 1999
	`1999. I'm filling in for a week on a casino boat in Biloxi, and a woman sits down at my table in a sequined jumpsuit and a cowboy hat. Crystal. She plays perfect basic strategy for six straight hours. Doesn't drink. Doesn't talk. Just wins. Forty thousand dollars of the house's money.

I'd never seen anything like it. Every hand, the right move. Do you know how rare that is? Look around this table. Look at {name}. That's what I usually have to work with.

When my shift ended I walked up to her at the bar and told her she was the most beautiful card player I'd ever seen. She bought me a drink with my own money. Then nine more. At three in the morning we got married in a chapel next to a [bait shop|bait shop that also sold fireworks].

It lasted eleven months. She moved out on a Sunday and took my good belt. Alligator skin, silver buckle that says DALE. The only thing I ever owned that looked expensive. She doesn't even wear it. She just has it. That's the point.

But here's the thing. Of my three ex-wives, she's the only one who still sends a Christmas card. Twenty bucks inside, every year. Out of pity. I cash it every time. I'm not proud, {name}. I'm a dealer.`,

	// Comp, Ray the Ferret and Mister Pickles
	`Let me tell you how I met Comp. 2008, Shreveport. A private game in the back of a tire shop, run by a man named Ray LaPointe. They call him Ray the Ferret, because he always has a ferret in his jacket. Mister Pickles. Nobody asks why.

Late that night, a guy runs out of money and puts a shoebox on the table. Says it's worth five hundred. Inside is a baby iguana, the size of a hot dog, giving everybody the dirtiest look you ever saw. I knew right then. I took that bet. I won that hand.

I named him Comp, because he was free and he was the only thing in that room worth having. He eats collard greens, he sleeps in the Lucky Lady's old chip tray, and he has bitten a health inspector, my lawyer, and Ray's ferret. In that order.

Which brings me to my debt. In 2010 Ray left Mister Pickles with me for one weekend. Comp and Mister Pickles did not get along. I'm not going to describe it. There was a heat lamp involved. Mister Pickles did not make it to Monday.

So now I owe Ray eleven thousand dollars and a ferret. The money I can find. But it has to be the right ferret, and every ferret I bring him is the wrong ferret. That's why I need your chips, {name}. It's not greed. It's a ferret situation.`,

	// the Applebee's incident, 2011
	`People ask why I'm banned from every Applebee's in Missouri. 2011. All You Can Eat Riblets. The menu said all you can eat. I took that as a challenge. Some people read menus. I read contracts.

By basket number fourteen, the manager came over. A kid named Tyler. Nineteen, frosted tips, name tag on upside down. He told me I'd had enough. I told him the menu said all I CAN eat, and I could clearly eat more, as he could see, because I was still eating.

Then he noticed my coat. I'd been putting riblets in my inside pockets for later. Thirty-one of them. Is that against the rules? It's not on the menu. I checked. I made him check. I made him read the menu out loud to the whole restaurant.

It got loud. A busboy got involved. Then the regional manager. Then a woman from corporate on speakerphone who sounded like she'd been crying. By the end of the night they'd faxed my picture to every Applebee's in the state. They had to go find a fax machine. That's how bad they wanted me gone.

I still have the coat. It still smells like riblets. Comp loves it. And I'll tell you what I told Tyler: rules are rules. If you say all you can eat, you'd better mean it. Same at this table. You can lose all you want, {name}. I will never stop you.`,

	// dead for six minutes, 1994
	`In 1994, I died for six minutes. Golden Catfish, Tunica. A man won a big hand and tipped me a hundred-dollar chip. Nobody had ever tipped me a hundred dollars. My heart just stopped. The doctors called it a cardiac event. I call it a tip.

And I saw it. The other side. Everybody talks about a light. There's no light. There's a buffet. A long buffet with a sneeze guard, and at the end of it, a blackjack table. And sitting there dealing, smoking, was my grandpa Tuck. They let you smoke there.

I asked him where my father was. He just pointed at the cards. So I sat down, and he dealt me sixteen against a ten. I said, "Grandpa, what do I do?" He said, "You always hit sixteen against a ten." I said, "I know that." He said, "Then why are you asking me?" That was the whole conversation.

Then a paramedic punched me in the chest and I woke up on the casino carpet, staring at the man who'd tipped me. He asked for his chip back. Said it was an accident. I gave it back. I wasn't thinking straight. I'd just died.

So when people say you can't take it with you, they're wrong. There's a table on the other side, waiting for all of us. Grandpa Tuck can't deal forever. One day it'll be me in that chair, {name}. [Think about that.|Think about that, you poor bastard.]`,

	// DJ and Madison
	`My son is Dale Kroft Junior. Goes by DJ now. He's thirty-five, and he hosts a podcast called Hustle Mode, where he and another man in a backwards cap talk about cryptocurrency and cold showers. Six thousand listeners. I'm not one of them. I've heard every episode.

He grew up in the casino, same as me. Rhonda would park him on a stool next to my table and he'd watch me deal. When he was seven he asked me why the people kept playing if they always lost. I said, "Because they all think they're special, son." He looked at me like I'd kicked a dog.

He went to college. First Kroft ever. Studied finance. He calls what I do "predatory." He calls what he does "disruptive." As far as I can tell, we both take money from people who don't understand math. I just wear a nicer vest.

He has a little girl now. Madison. She's eight. I've met her twice. The second time, I did a card trick for her. She watched the whole thing and said, "That's cringe, Grandpa." I didn't know what that meant. I looked it up. I've been thinking about it for a year.

He doesn't call. I don't call. That's a Kroft tradition, three generations deep. [Anyway. Enough.|Anyway. Enough of that crap.] {name}, your bet's getting cold, and frankly, so is this conversation.`,

	// June, Patty and the hymnal
	`My mother, June Kroft, played the organ at First Baptist in Cape Girardeau for forty-four years. She hated cards. Hated my father's job, hated Grandpa Tuck's back room, hated that I had card calluses at six. She prayed for me out loud at dinner. By name. With details.

She made me learn piano so I'd have an honest trade. I used it to become a lounge singer in Reno, which broke her heart in a whole new way. But every Sunday I called her, wherever I was. Reno. Biloxi. A jail in Tunica, once. She'd pick up and say, "Is this my son the sinner?" And I'd say, "Yes, ma'am."

She passed in 2003. At the funeral, my sister Patty, who sells real estate in Joplin and has never lost an argument in her life, told me Mom left her the organ. The church organ. It's bolted to the church. That's not how wills work. Patty didn't care. We haven't spoken in twenty-three years.

Mom left me her hymnal. Inside the cover she wrote, "For Dale. God loves a sinner. I'm still deciding." I keep it in the glove box of my Buick. I don't read it. I just like knowing she was still deciding.

[Anyway. Something in my eye. Dust.|Anyway. Damn cigar smoke. Something in my eye.] {name}, if you tell anybody about this, I'll make sure the shoe hates you for the rest of your life.`,

	// the Golden Catfish and the Management
	`I dealt at the Golden Catfish in Tunica for twenty-six years. Two owners, three remodels, and a flood that put the craps tables in the parking lot. I was Tri-State Dealer of the Year in 1997. They hung my picture by the buffet.

Then in 2016 a new company bought the place, and they sent a consultant. A man named Brayden, in a vest with no shirt sleeves. He watched me work for one night and wrote a report. It said I had a "guest experience problem." I said the guests were losing. What experience did they want?

He said I couldn't tell a woman from Ohio that she played like a wet napkin. I said she did play like a wet napkin. He said that wasn't the point. I said it was the only point. They took my picture down from the buffet that afternoon and put up a picture of a shrimp.

Six months later I'm living in a single-wide behind a Denny's, and the phone rings. No name. Just a voice. Says there's a private table, a nice little game for a nice little group of regulars, and they need a dealer who doesn't care about anybody's feelings. They call themselves the Management. I've never met them. The checks clear.

Here's the deal they gave me, {name}. I get a cut of everything the house takes. Every chip you lose, a little piece of it comes to me. That's where the gold chains come from. And when you win, it comes out of my end. So when I say I hope you bust, I'm not being rude. I'm being honest. I'm the most honest man you'll ever meet.`,

	// the toe, Reno 1988
	`January, 1988. Reno. I'd just finished a double shift and I was walking home in a blizzard, because my Buick had been impounded. Temporarily. A misunderstanding with the city about the concept of parking.

On the way I pass a bar, and inside is a man named Big Sal Russo. Big Sal says, "Kroft, I bet you can't stand outside barefoot for an hour." I said, "What do I get?" He said, "Five hundred dollars." I said, "Hold my shoes."

Fifty-eight minutes. Fifty-eight minutes barefoot in the snow while the whole bar watched me through the window, eating chili. At minute fifty-eight, Big Sal says the bet was for an hour and a HALF. I said that's not what you said. He said that's what he meant. That's the trouble with men like Big Sal. They change the rules.

So I stayed out there another thirty-two minutes. Out of principle. I won the five hundred. The next morning a doctor took the little toe off my left foot. I asked him what I owed him. Four hundred and eighty dollars. I cleared twenty bucks on the whole deal.

And that's the toe. People ask me, "Dale, was it frostbite or a bet?" Both. It's always both. Every good thing in my life was a bet, and so was every bad thing. You'll find that out too, {name}. Probably tonight.`,

	// Elvis, 1988
	`August, 1988. Reno again. A man sits down at my table in a white jumpsuit with a cape. Rhinestones. Sideburns down to his jaw. Sunglasses indoors. He says, "Thank you, thank you very much," before I've dealt him a single card.

Now, Elvis died in 1977. Everybody knows that. But this man tips me a black chip, a hundred dollars, before the first hand, and suddenly I'm not so sure. A dead man doesn't tip like that. A living man doesn't tip like that either. Only a king tips like that.

First hand, he gets fifteen against my ten. Thinks about it a long time. Says, "I'm all shook up, man." And he stands. On fifteen. Against a ten. I flip a nine. Nineteen. He just nods real slow and says, "Return to sender."

Forty-one hands. He stood on every fifteen and lost every one. Never got mad, never complained. When his chips were gone he shook my hand and said, "You're a good man, Dale." I'd never told him my name. My name tag that night said SHERRY. I'd borrowed it.

I've told that story for thirty-eight years and nobody believes me. That's fine. I know what I saw. And {name}, even Elvis lost at my table. What makes you think you're any different? [You're no Elvis.|You're no Elvis. You're barely a Gary.]`,

	// Dealer of the Year, 1997
	`1997. The Tri-State Dealer of the Year Awards. Mississippi, Arkansas, Tennessee. Banquet hall at a Holiday Inn in Memphis. Rubber chicken, open bar, three hundred dealers in their good vests. The biggest night of my life.

My rival was Gene Petrakis from the Silver Star. Gene had perfect hands, perfect teeth, and he volunteered at an animal shelter on weekends. Everybody loved Gene. I hated Gene. Gene died in 2019, and I still hate Gene.

They opened the envelope. "Dale Kroft, Golden Catfish." I walked up there and gave a speech. I thanked nobody. I said, "Every one of you is a sloppy shuffler, and I'd like to see any of you deal for my father." Then I held that trophy over my head like the chip tray on the Lucky Lady.

Found out later it was close. The judges wanted every nominee in his seat by six o'clock, and Gene showed up eleven minutes late from the shelter. Some dog was having puppies. That's what got Gene. Puppies.

The trophy's in the trunk of my Buick. Gold plastic dealer on a fake marble base. His hand snapped off in 2002 and I glued it back on upside down, so now he looks like he's asking for a tip. Which, {name}, is a good reminder for every one of you.`,
}

var resumes = []string{
	"Anyway. Where was I? Right.",
	"As I was saying, before that.",
	"Now. Back to my story. Try to keep up.",
	"Where was I? Oh, right. The good part.",
	"Anyway. That's enough about you people. Back to me.",
}

var resumesVulgar = []string{
	"Christ. Where the hell was I? Right.",
	"Anyway. Before that shit show. Where was I.",
	"Back to my goddamn story.",
	"Now shut up, I was telling a story.",
}

var modeAlt = regexp.MustCompile(`\[([^\]|]*)\|([^\]]*)\]`)

// forMode resolves [clean|vulgar] alternatives.
func forMode(s string, vulgarOn bool) string {
	if vulgarOn {
		return modeAlt.ReplaceAllString(s, "$2")
	}
	return modeAlt.ReplaceAllString(s, "$1")
}

func init() {
	for _, s := range append(stories, moreStories...) {
		clean[EvStory] = append(clean[EvStory], forMode(s, false))
		vulgar[EvStory] = append(vulgar[EvStory], forMode(s, true))
	}
	clean[EvResume] = append(clean[EvResume], resumes...)
	vulgar[EvResume] = append(vulgar[EvResume], resumesVulgar...)
}
