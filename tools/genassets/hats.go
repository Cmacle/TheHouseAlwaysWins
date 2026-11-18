// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package main

import (
	"image/color"
)

// Hat sprites: each cell is 32x20. The bottom 6 rows overlap the top of the
// 32x32 avatar. Order must match the Sprite indexes in internal/cosmetics.

const hatW, hatH = 32, 20

type sprite struct {
	rows []string
	pal  map[rune]string
}

var hatSprites = []sprite{
	// 0 party hat
	{[]string{
		".....w.....",
		"....www....",
		".....k.....",
		"....kpk....",
		"....kyk....",
		"...kpppk...",
		"...kyyyk...",
		"..kpppppk..",
		"..kyyyyyk..",
		".kpppppppk.",
		".kyyyyyyyk.",
		"kpppppppppk",
		"kkkkkkkkkkk",
	}, map[rune]string{'p': "e83a7a", 'y': "ffd23f", 'w': "ffffff"}},
	// 1 ball cap
	{[]string{
		".....kkkkkk.......",
		"...kkbbbbbbkk.....",
		"..kbbbbwbbbbbk....",
		".kbbbbbbbbbbbbk...",
		".kbbbbbbbbbbbbbk..",
		"kbbbbbbbbbbbbbbkkk",
		"kddddddddddddddddk",
		".kkkkkkkkkkkkkkkk.",
	}, map[rune]string{'b': "2f6fd6", 'd': "1d4a99", 'w': "ffffff"}},
	// 2 beanie
	{[]string{
		".......kkk.......",
		"......kwwwk......",
		".......kkk.......",
		"....kkrrrrrkk....",
		"...krrrrrrrrrk...",
		"..krrrrrrrrrrrk..",
		"..kRRRRRRRRRRRk..",
		"..kRrRrRrRrRrRk..",
		"..kkkkkkkkkkkkk..",
	}, map[rune]string{'r': "c8323c", 'R': "8e1f28", 'w': "f4f4f0"}},
	// 3 propeller beanie
	{[]string{
		"yyyyyy.k.yyyyyy",
		".yyyy.kak.yyyy.",
		"......kak......",
		"....kkrgbkk....",
		"...krrrgbbbk...",
		"..krrrrgbbbbk..",
		"..krrrrgbbbbk..",
		"..kyyyyyyyyyk..",
		"..kkkkkkkkkkk..",
	}, map[rune]string{'y': "ffd23f", 'a': "bbbbbb", 'r': "e04848", 'g': "3fb68b", 'b': "3a7bff"}},
	// 4 cowboy hat
	{[]string{
		"........kkkkkkkk........",
		".......kttttkttttk......",
		"......kttttttttttttk....",
		"......kttttttttttttk....",
		"......kBBBBBBBBBBBBk....",
		"kk....kttttttttttttk..kk",
		"ktkkkkkttttttttttttkkktk",
		".kttttttttttttttttttttk.",
		"..kkkkkkkkkkkkkkkkkkkk..",
	}, map[rune]string{'t': "a0703a", 'B': "3a2414"}},
	// 5 tin foil hat
	{[]string{
		"......k......",
		".....kwk.....",
		"....kssgk....",
		"...kgsswsk...",
		"...ksgssgk...",
		"..ksswsgssk..",
		"..kgssgswgk..",
		".ksgswssgssk.",
		".kgssgsswsgk.",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'s': "b8bcc8", 'g': "7d828f", 'w': "f4f6fb"}},
	// 6 sombrero
	{[]string{
		"..........kkkkkkkk..........",
		".........kyyyyyyyyk.........",
		"........kyyyyyyyyyyk........",
		"........kyyyyyyyyyyk........",
		"........krgrgrgrgrgk........",
		".kkkkkkkkyyyyyyyyyyykkkkkkk.",
		"kyyyyyyyyyyyyyyyyyyyyyyyyyyk",
		"kyrgyrgyrgyrgyrgyrgyrgyrgyyk",
		".kkkkkkkkkkkkkkkkkkkkkkkkkk.",
	}, map[rune]string{'y': "e8c25a", 'r': "c8323c", 'g': "2e9d4f"}},
	// 7 traffic cone
	{[]string{
		"......kk......",
		".....kook.....",
		".....kwwk.....",
		"....kwwwwk....",
		"....kooook....",
		"...kooooook...",
		"...kwwwwwwk...",
		"..kwwwwwwwwk..",
		"..koooooooook.",
		".kkkkkkkkkkkkk",
		"kooooooooooook",
		"kkkkkkkkkkkkkk",
	}, map[rune]string{'o': "ff7a1a", 'w': "f4f4f0"}},
	// 8 top hat
	{[]string{
		"...kkkkkkkkkk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kRRRRRRRRk...",
		"...kKKKKKKKKk...",
		"kkkkKKKKKKKKkkkk",
		"kKKKKKKKKKKKKKKk",
		".kkkkkkkkkkkkkk.",
	}, map[rune]string{'K': "2a2a33", 'h': "4f4f60", 'R': "c62a3d"}},
	// 9 fez
	{[]string{
		"....kkkkkkk..",
		"...krrrrrrrkk",
		"...krrrrrrrkY",
		"..krrrrrrrrkY",
		"..krrrrrrrrkY",
		".krrrrrrrrrkY",
		".krrrrrrrrrk.",
		"kRRRRRRRRRRRk",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'r': "c8203a", 'R': "8e1726", 'Y': "ffd23f"}},
	// 10 pirate hat
	{[]string{
		".....kkkkkkkk.....",
		"...kkKKKKKKKKkk...",
		"..kKKKKKwwKKKKKk..",
		".kKKKKKwwwwKKKKKk.",
		"kKKKKKKKwwKKKKKKKk",
		"kKKKKKKwKKwKKKKKKk",
		"kyyyyyyyyyyyyyyyyk",
		".kkkkkkkkkkkkkkkk.",
	}, map[rune]string{'K': "1d1d26", 'w': "f4f4f0", 'y': "d9a227"}},
	// 11 dunce cap
	{[]string{
		"......k......",
		".....kwk.....",
		".....kwk.....",
		"....kwwwk....",
		"....kwwwk....",
		"...kwkkwwk...",
		"...kwkwkwk...",
		"..kwwkwkwwk..",
		"..kwwkkwwwk..",
		".kwwwwwwwwwk.",
		".kbbbbbbbbbk.",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'w': "f4f4f0", 'b': "3a7bff"}},
	// 12 viking helmet
	{[]string{
		"w..............w",
		"wk............kw",
		"wwk...kkkk...kww",
		".wwk.kssssk.kww.",
		"..wwksshsssskww.",
		"...ksshssssssk..",
		"..kssshsssssssk.",
		"..kyyyyyyyyyyyk.",
		"..kyryyryyryyrk.",
		"..kkkkkkkkkkkkk.",
	}, map[rune]string{'w': "f0e6c8", 's': "9aa0ab", 'h': "d8dde6", 'y': "c8a040", 'r': "8a5a12"}},
	// 13 halo (floats)
	{[]string{
		"....kkkkkkkk....",
		"..kkyyyyyyyykk..",
		".kyyk......kyyk.",
		".kyyk......kyyk.",
		"..kkyyyyyyyykk..",
		"....kkkkkkkk....",
		"................",
		"................",
		"................",
		"................",
		"................",
	}, map[rune]string{'y': "ffe066"}},
	// 14 devil horns
	{[]string{
		"k........................k",
		"rk......................kr",
		"rrk....................krr",
		"rrrk..................krrr",
		".rRrk................krRr.",
		".rRRrk..............krRRr.",
		"..rRRrk............krRRr..",
		"...rRRk............kRRr...",
	}, map[rune]string{'r': "e0283c", 'R': "8e1726"}},
	// 15 wizard hat
	{[]string{
		"...........kk....",
		"..........kbbk...",
		".........kbbk....",
		"........kbbbk....",
		".......kbbybk....",
		"......kbbbbbk....",
		"......kbybbbk....",
		".....kbbbbbybk...",
		"....kbbbbbbbbk...",
		"...kbbybbbbbbbk..",
		"...kyyyyyyyyyyk..",
		".kkbbbbbbbbbbbbkk",
		"kbbbbbbbbbbbbbbbk",
		".kkkkkkkkkkkkkkk.",
	}, map[rune]string{'b': "3b3fb8", 'y': "ffd23f"}},
	// 16 unicorn horn
	{[]string{
		".....k.....",
		"....kwk....",
		"....kpk....",
		"...kwwpk...",
		"...kppwk...",
		"..kwwppwk..",
		"..kppwwpk..",
		".kwwppwwpk.",
		".kppwwppwk.",
		"kkkkkkkkkkk",
	}, map[rune]string{'w': "fff6fb", 'p': "f59ad9"}},
	// 17 graduation cap
	{[]string{
		".......kk.......",
		".....kkKKkk.....",
		"...kkKKKKKKkk...",
		".kkKKKKKKKKKKkk.",
		"kKKKKKKyKKKKKKKk",
		".kkKKKKyKKKKKkk.",
		"...kkKKyKKKkk...",
		"....kKKyKKKk....",
		"....kKKyKKKkY...",
		"....kKKKKKKkY...",
		"....kkkkkkkkY...",
	}, map[rune]string{'K': "23232f", 'y': "ffd23f", 'Y': "ffd23f"}},
	// 18 golden top hat
	{[]string{
		"...kkkkkkkkkk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGGGGGGGDk...",
		"...kRRrRRrRRk...",
		"...kGGGGGGGDk...",
		"kkkkGGGGGGGGkkkk",
		"kGGGGGGGGGGGGGDk",
		".kkkkkkkkkkkkkk.",
	}, map[rune]string{'G': "f2c14e", 'D': "a8741a", 'w': "fff2b8", 'R': "c62a3d", 'r': "7fe9ff"}},
	// 19 diamond tiara
	{[]string{
		".........c.........",
		"........cWc........",
		"...c.....c.....c...",
		"..cWc...kSk...cWc..",
		"...c...kSSSk...c...",
		"...k..kSScSSk..k...",
		"..kSkkSSSSSSSkkSk..",
		".kSSSSSSSSSSSSSSSk.",
		".kSpSSScSSSpSSScSk.",
		"..kkkkkkkkkkkkkkk..",
	}, map[rune]string{'c': "7fe9ff", 'W': "ffffff", 'S': "d8dde6", 'p': "ff5c8a"}},
	// 20 hair on fire
	{[]string{
		"....r.....r.......",
		"...ro....ror...r..",
		"...roo..rooo..ror.",
		"..rooy.roooy.rooo.",
		"..royyrooyyyrooyy.",
		".royyyyoyyyyyoyyyo",
		".ryyyyyyWyyyyyyyyr",
		"royyyWyyyyyyWyyyor",
		"rrooooooooooooooor",
	}, map[rune]string{'r': "c8203a", 'o': "ff7a1a", 'y': "ffd23f", 'W': "fff6c8"}},
	// 21 space helmet
	{[]string{
		"...........R...........",
		"...........k...........",
		"........kkkkkkk........",
		".....kkkcccccccckkk....",
		"...kkcccWWcccccccccckk.",
		"..kcccWWcccccccccccccck",
		".kcccWccccccccccccccccc",
		".kccccccccccccccccccccc",
		"ksssssssssssssssssssssk",
		"kskkskkskkskkskkskkssk.",
	}, map[rune]string{'R': "ff3030", 'c': "9fd8ffa0", 'W': "ffffffd0", 's': "c8ccd6"}},
	// 22 crown
	{[]string{
		"....R.......R.......R....",
		"...kyk.....kyk.....kyk...",
		"...kyyk...kyyyk...kyyk...",
		"..kyyyyk.kyyyyyk.kyyyyk..",
		"..kyyyyykyyyyyyykyyyyyk..",
		"..kyyyyyyyyyyyyyyyyyyyk..",
		"..kyyByyyyyRyyyyyyByyyk..",
		"..kyBBByyyRRRyyyyBBByyk..",
		"..kyyByyyyyRyyyyyyByyyk..",
		"..kYYYYYYYYYYYYYYYYYYYk..",
		"..kkkkkkkkkkkkkkkkkkkkk..",
	}, map[rune]string{'y': "f2c14e", 'Y': "b07a18", 'R': "e0283c", 'B': "3a7bff"}},
	// 23 laurel wreath (gold)
	{[]string{
		".gg....................gg.",
		"gGgg..................ggGg",
		".gGgg................ggGg.",
		"..ggGg..............gGgg..",
		"...gggGg..........gGggg...",
		".....ggggg......ggggg.....",
		".......gggg....gggg.......",
	}, map[rune]string{'g': "f2c14e", 'G': "a8741a"}},
	// 24 the nightcap (most hands played): a striped sleeping cap that
	// flops over to one side, with a pom-pom and a little gold star
	{[]string{
		"........................ww...",
		".......................wwww..",
		"........kkkkkkkkk......wwww..",
		"......kkbbBBbbBBbkk.....ww...",
		".....kbbBBbbBBbbBBbk...kk....",
		"....kbBBbbBBbbBBbbBBkkkbk....",
		"....kbbBBbbyBBbbBBbbBBbk.....",
		"...kbBBbbByyybbBBbbBBbk......",
		"...kbbBBbbBybBBbbBBbkk.......",
		"..kbBBbbBBbbBBbbBBbk.........",
		"..kbbBBbbBBbbBBbbBBk.........",
		".kwwwwwwwwwwwwwwwwwwk........",
		".kWwWwWwWwWwWwWwWwWwk........",
		"..kkkkkkkkkkkkkkkkkk.........",
	}, map[rune]string{'b': "3a4fb8", 'B': "6f86ea", 'y': "ffd23f", 'w': "f4f2ea", 'W': "c9c6ba"}},
	// 25 the chip tower (biggest bet): a wobbly stack of chips, gold on top
	{[]string{
		"....kkkkkkkk....",
		"...kgGgggGggk...",
		"...kyywyywyyk...",
		"....kkkkkkkk....",
		"...kprwrrwrrk...",
		"....kkkkkkkkk...",
		"....kbwbbwbbbk..",
		"....kkkkkkkkk...",
		"...kpwppwppwk...",
		"...kkkkkkkkkk...",
		"..knwnnwnnwnk...",
		"..kkkkkkkkkkk...",
		"...kgwggwggwgk..",
		"...kkkkkkkkkkk..",
		"..krwrrwrrwrrk..",
		"..kkkkkkkkkkkk..",
		".kkbwbbwbbwbbkk.",
		"kkkkkkkkkkkkkkkk",
	}, map[rune]string{'g': "f2c14e", 'G': "fff2b8", 'y': "c9961f", 'w': "f4f2ea",
		'r': "d2323c", 'p': "8a3fd1", 'b': "2f6fd6", 'n': "1f8a4c"}},
	// 26 green visor (poker dealer's eyeshade)
	{[]string{
		"....kkkkkkkkkkkkkkkkkk....",
		"...kggggggggggggggggggk...",
		"..kgGGGGGGGGGGGGGGGGGGgk..",
		".kgGGGGGGGGGGGGGGGGGGGGgk.",
		"kgggggggggggggggggggggggk",
		"kkkkkkkkkkkkkkkkkkkkkkkkk",
		".kwk..................kwk",
		".kwk..................kwk",
	}, map[rune]string{'g': "1f8a4c", 'G': "57d38c", 'w': "e8e4d8"}},
	// 27 sailor cap (flush)
	{[]string{
		".....kkkkkkkkkkkk.....",
		"...kkwwwwwwwwwwwwkk...",
		"..kwwwwwwwwwwwwwwwwk..",
		".kwwwwwwwwaawwwwwwwwk.",
		".kwwwwwwwwaawwwwwwwwk.",
		"kWWWWWWWWWWWWWWWWWWWWk",
		"kbbbbbbbbbbbbbbbbbbbbk",
		"kbbbbbbbbbbbbbbbbbbbbk",
		".kkkkkkkkkkkkkkkkkkkk.",
	}, map[rune]string{'w': "f6f6f2", 'W': "c9cbd0", 'b': "1d2f6e", 'a': "f2c14e"}},
	// 28 fedora (full house)
	{[]string{
		".......kkkkkkkkkk.......",
		"......kfffffkffffk......",
		".....kfffffkfffffk......",
		".....kfffffffffffk......",
		".....kfffffffffffk......",
		".....kbbbbbbbbbbbk......",
		"..kkkfffffffffffffkkk...",
		".kffffffffffffffffffffk.",
		"kffffffffffffffffffffffk",
		".kkkkkkkkkkkkkkkkkkkkkk.",
	}, map[rune]string{'f': "6b5b4a", 'b': "1a1a22"}},
	// 29 jester hat (bluffs)
	{[]string{
		"y....................y",
		"ky..................yk",
		".kr................pk.",
		".krr..............ppk.",
		"..krr....kyk.....ppk..",
		"..krrr..kpppk...pppk..",
		"...krrrkpppppkrrrrk...",
		"...krrrrpppppprrrrk...",
		"....kyyyyyyyyyyyyk....",
		"....kkkkkkkkkkkkkk....",
	}, map[rune]string{'r': "d2323c", 'p': "8a3fd1", 'y': "ffd23f"}},
	// 30 bowler hat (four of a kind)
	{[]string{
		"........kkkkkkkk........",
		"......kkbbbbbbbbkk......",
		".....kbbbBbbbbbbbbk.....",
		"....kbbbBbbbbbbbbbbk....",
		"....kbbbbbbbbbbbbbbk....",
		"....krrrrrrrrrrrrrrk....",
		"..kkbbbbbbbbbbbbbbbbkk..",
		".kbbbbbbbbbbbbbbbbbbbbk.",
		".kkkkkkkkkkkkkkkkkkkkkk.",
	}, map[rune]string{'b': "26262e", 'B': "55555f", 'r': "8e1f28"}},
	// 31 shark fin (beat Dale)
	{[]string{
		"......kk.......",
		".....kssk......",
		".....ksssk.....",
		"....kssssk.....",
		"....ksssssk....",
		"...kssssssk....",
		"...ksssssssk...",
		"..kssssssssk...",
		"..kSSSSSSSSSk..",
		".kSSSSSSSSSSSk.",
		"kkkkkkkkkkkkkkk",
	}, map[rune]string{'s': "6f8ea8", 'S': "e8eef2"}},
	// 32 pharaoh's headdress (straight flush)
	{[]string{
		"..........kkkkkk..........",
		"........kkggggggkk........",
		".......kgbbgggbbggk.......",
		"......kggggg..ggggggk.....",
		".....kbbbbbbbbbbbbbbbk....",
		"....kggggggggggggggggk....",
		"....kbbbbbbbbbbbbbbbbbk...",
		"...kgggggk.......kggggk...",
		"...kbbbbbk.......kbbbbk...",
		"..kgggggk.........kggggk..",
		"..kbbbbbk.........kbbbbk..",
		"..kkkkkk...........kkkkk..",
	}, map[rune]string{'g': "f2c14e", 'b': "2f4fbf"}},
	// 33 sultan's turban (royal flush)
	{[]string{
		"...........kk.........",
		"..........kwwk........",
		"..........kwwk........",
		".......kkkkrrkkkk.....",
		".....kkwwwwkrkwwwkk...",
		"....kwwwwwwwkwwwwwwk..",
		"...kWWWWWWWWWWWWWWWWk.",
		"...kwwwwwwwkgkwwwwwwk.",
		"...kWWWWWWkgrgkWWWWWk.",
		"...kwwwwwwwkgkwwwwwwk.",
		"...kWWWWWWWWWWWWWWWWk.",
		"....kkkkkkkkkkkkkkkk..",
	}, map[rune]string{'w': "f4efe0", 'W': "d8d0b8", 'r': "e0283c", 'g': "f2c14e"}},
	// 34 the moneybags (most chips ever held): a stuffed sack, tied with
	// gold cord, a bill sticking out and coins spilling off the brim
	{[]string{
		"..........kk...kk.......",
		".........kGgk.kgGk......",
		"..........kgkkgk........",
		"......kkk..kyyk..kk.....",
		".....knnnk.kyyk.kGgk....",
		".....knNnkkyyyykkk......",
		"......kkkbbbbbbbbk......",
		".....kbbbbBbbbbbbbk.....",
		"....kbbBbbbbkkkbbbbk....",
		"...kbbbbbbbkgggkbbbbk...",
		"...kbbbbbbkgkbbbbbbbk...",
		"..kbbbbBbbbkgggkbbbbbk..",
		"..kbbbbbbbbbbbkgkbbbbk..",
		"..kbbbBbbbbkgggkbbBbbk..",
		"..kbbbbbbbbbbkbbbbbbbk..",
		"...kbbbbbbbbbbbbbbbbk...",
		".kk.kkbbbbbbbbbbbbkk.kk.",
		"kGgk..kkkkkkkkkkkk..kGgk",
		".kk..................kk.",
	}, map[rune]string{'b': "a07a48", 'B': "c8a06a", 'y': "f2c14e", 'g': "f2c14e", 'G': "fff2b8",
		'n': "3f9a52", 'N': "8fe0a0"}},
}

func genHats() {
	cols := 8
	rows := (len(hatSprites) + cols - 1) / cols
	img := newImg(cols*hatW, rows*hatH)
	for i, sp := range hatSprites {
		ox, oy := (i%cols)*hatW, (i/cols)*hatH
		w := 0
		for _, r := range sp.rows {
			if len(r) > w {
				w = len(r)
			}
		}
		x0 := ox + (hatW-w)/2
		y0 := oy + hatH - len(sp.rows)
		for j, r := range sp.rows {
			for k, ch := range r {
				var c color.NRGBA
				switch ch {
				case '.':
					continue
				case 'k':
					c = hex("15151a")
				default:
					h, ok := sp.pal[ch]
					if !ok {
						continue
					}
					c = hex(h)
				}
				set(img, x0+k, y0+j, c)
			}
		}
	}
	save("hats.png", img)
}
