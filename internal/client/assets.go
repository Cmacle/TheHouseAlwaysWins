// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
)

//go:embed defaultassets/*
var defaultFS embed.FS

// Theme holds sizes and colours that can be overridden with assets/theme.json.
type Theme struct {
	CardWidth      int     `json:"card_width"`
	CardHeight     int     `json:"card_height"`
	CardFanStep    int     `json:"card_fan_step"`
	ChipSize       int     `json:"chip_size"`
	ChipValues     []int64 `json:"chip_values"`
	FontCellWidth  int     `json:"font_cell_width"`
	FontCellHeight int     `json:"font_cell_height"`
	FontColumns    int     `json:"font_columns"`
	FontFirstChar  int     `json:"font_first_char"`
	FontSpaceWidth int     `json:"font_space_width"`
	FontMonospace  bool    `json:"font_monospace"`
	NineSliceEdge  int     `json:"nine_slice_border"`
	HatWidth       int     `json:"hat_width"`
	HatHeight      int     `json:"hat_height"`
	HatOverlap     int     `json:"hat_overlap"` // rows of the hat drawn over the avatar
	FrameSize      int     `json:"frame_size"`
	DealerCellW    int     `json:"dealer_cell_width"`
	DealerCellH    int     `json:"dealer_cell_height"`
	CigarX         int     `json:"cigar_x"` // cigar tip inside a vulgar dealer cell (smoke origin)
	CigarY         int     `json:"cigar_y"`
	// Where the dealer's mouth opens while talking, inside a dealer cell:
	// [x, y, width] for the regular row and the vulgar row.
	Mouth     [2][3]int         `json:"mouth"`
	PanelEdge int               `json:"panel_border"`
	Colors    map[string]string `json:"colors"`
	colors    map[string]color.RGBA
}

func defaultTheme() Theme {
	return Theme{
		CardWidth: 24, CardHeight: 32, CardFanStep: 9, ChipSize: 20,
		ChipValues:    []int64{5, 10, 25, 100, 500, 1000},
		FontCellWidth: 6, FontCellHeight: 8, FontColumns: 16, FontFirstChar: 32, FontSpaceWidth: 4,
		NineSliceEdge: 4, PanelEdge: 5,
		HatWidth: 32, HatHeight: 20, HatOverlap: 6, FrameSize: 36, DealerCellW: 48, DealerCellH: 48, CigarX: 41, CigarY: 26,
		Mouth: [2][3]int{{22, 28, 5}, {20, 28, 6}},
		Colors: map[string]string{
			"text":       "#f3ead2",
			"text_dim":   "#9fb8a4",
			"gold":       "#f2c14e",
			"win":        "#7dff8a",
			"lose":       "#ff6b6b",
			"push":       "#9ecbff",
			"felt_text":  "#57b97c",
			"highlight":  "#ffe066",
			"shadow":     "#000000",
			"seat_open":  "#2f8a52",
			"overlay":    "#000000a0",
			"row_alt":    "#ffffff10",
			"row_me":     "#f2c14e30",
			"error":      "#ff8a7a",
			"button_txt": "#fff4dc",
			"online":     "#7dff8a",
			"bubble":     "#fbf7ea",
			"bubble_txt": "#1a1a22",
			"smoke":      "#c8c8d0b0",
		},
	}
}

// Assets are all images and sounds used by the client.
type Assets struct {
	Dir   string
	Theme Theme
	Font  *Font

	Table, Cards, CardBack, Chips, Dealers, Shoe, Logo, DefaultAvatar *image.RGBA
	TableCustom                                                       bool // table.png was replaced: stretch it to fit
	Button, ButtonHover, ButtonDown, ButtonDisabled, ButtonGold       *image.RGBA
	ButtonGoldHover, Field, FieldFocus, Panel, Checkbox, Arrow        *image.RGBA
	Hats, Frames                                                      *image.RGBA

	Sounds map[string][]byte
	Icon   []byte // raw PNG for the window icon
}

// SoundNames lists the sound files the client knows about.
var SoundNames = []string{"card", "chip", "click", "win", "blackjack", "lose", "push", "bust", "turn", "refill", "shuffle", "error",
	"talk1", "talk2", "talk3", "talk4", "talklow1", "talklow2", "talklow3", "talklow4",
	"chat", "rev_bust", "rev_safe", "rev_meh", "rev_good", "rev_great", "rev_21",
	"rev_dealer_bust", "rev_dealer_strong", "rev_dealer_ok", "rev_dealer_more"}

func init() {
	for _, st := range SuspenseStyles {
		for _, l := range SuspenseLengths {
			SoundNames = append(SoundNames, fmt.Sprintf("suspense_%s_%d", st, l))
		}
	}
}

// ExportDefaults writes every default asset that does not yet exist in dir, so
// players can see and replace them.
func ExportDefaults(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, _ := fs.ReadDir(defaultFS, "defaultassets")
	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, _ := defaultFS.ReadFile("defaultassets/" + e.Name())
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	themePath := filepath.Join(dir, "theme.json")
	if _, err := os.Stat(themePath); errors.Is(err, os.ErrNotExist) {
		b, _ := json.MarshalIndent(defaultTheme(), "", "  ")
		_ = os.WriteFile(themePath, b, 0o644)
	}
	readme := filepath.Join(dir, "README.txt")
	if _, err := os.Stat(readme); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(readme, []byte(assetReadme), 0o644)
	}
	return nil
}

func (a *Assets) read(name string) ([]byte, error) {
	if a.Dir != "" {
		if b, err := os.ReadFile(filepath.Join(a.Dir, name)); err == nil {
			return b, nil
		}
	}
	return defaultFS.ReadFile("defaultassets/" + name)
}

func (a *Assets) image(name string) *image.RGBA {
	b, err := a.read(name)
	var img image.Image
	if err == nil {
		img, err = png.Decode(bytes.NewReader(b))
	}
	if err != nil {
		// fall back to the embedded copy if a replacement is broken
		if def, derr := defaultFS.ReadFile("defaultassets/" + name); derr == nil {
			img, err = png.Decode(bytes.NewReader(def))
		}
	}
	if err != nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	return toRGBA(img)
}

func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// LoadAssets loads assets from dir, falling back to the embedded defaults for
// anything missing.
func LoadAssets(dir string) (*Assets, error) {
	a := &Assets{Dir: dir, Theme: defaultTheme(), Sounds: map[string][]byte{}}
	if b, err := a.read("theme.json"); err == nil {
		t := defaultTheme()
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("theme.json: %w", err)
		}
		// keep defaults for colours not listed
		for k, v := range defaultTheme().Colors {
			if _, ok := t.Colors[k]; !ok {
				if t.Colors == nil {
					t.Colors = map[string]string{}
				}
				t.Colors[k] = v
			}
		}
		a.Theme = t
	}
	a.Theme.colors = map[string]color.RGBA{}
	for k, v := range a.Theme.Colors {
		a.Theme.colors[k] = parseHex(v)
	}
	if len(a.Theme.ChipValues) == 0 {
		a.Theme.ChipValues = defaultTheme().ChipValues
	}
	a.Table = a.image("table.png")
	if b, err := a.read("table.png"); err == nil && !isDefaultAsset("table.png", b) {
		a.TableCustom = true
	}
	a.Cards = a.image("cards.png")
	a.CardBack = a.image("card_back.png")
	a.Chips = a.image("chips.png")
	a.Dealers = a.image("dealers.png")
	a.Shoe = a.image("shoe.png")
	a.Logo = a.image("logo.png")
	a.DefaultAvatar = a.image("default_avatar.png")
	a.Button = a.image("button.png")
	a.ButtonHover = a.image("button_hover.png")
	a.ButtonDown = a.image("button_down.png")
	a.ButtonDisabled = a.image("button_disabled.png")
	a.ButtonGold = a.image("button_gold.png")
	a.ButtonGoldHover = a.image("button_gold_hover.png")
	a.Field = a.image("field.png")
	a.FieldFocus = a.image("field_focus.png")
	a.Panel = a.image("panel.png")
	a.Checkbox = a.image("checkbox.png")
	a.Arrow = a.image("arrow.png")
	a.Hats = a.image("hats.png")
	a.Frames = a.image("frames.png")
	a.Font = newFont(a.image("font.png"), a.Theme)
	a.Icon, _ = a.read("icon.png")
	for _, s := range SoundNames {
		if b, err := a.read(s + ".wav"); err == nil {
			a.Sounds[s] = b
		}
	}
	return a, nil
}

// C returns a theme colour.
func (a *Assets) C(name string) color.RGBA {
	if c, ok := a.Theme.colors[name]; ok {
		return c
	}
	return color.RGBA{255, 0, 255, 255}
}

func parseHex(s string) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	var r, g, b, al uint8 = 0, 0, 0, 255
	switch len(s) {
	case 6:
		fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	case 8:
		fmt.Sscanf(s, "%02x%02x%02x%02x", &r, &g, &b, &al)
	}
	// premultiply for image.RGBA
	return color.RGBA{uint8(uint16(r) * uint16(al) / 255), uint8(uint16(g) * uint16(al) / 255), uint8(uint16(b) * uint16(al) / 255), al}
}

// CardImage returns the sprite for a card (face down if hidden).
func (a *Assets) CardImage(c bj.Card) *image.RGBA {
	if c.Hidden() || c.Rank > 13 || c.Suit > 3 {
		return a.CardBack
	}
	w, h := a.Theme.CardWidth, a.Theme.CardHeight
	x, y := int(c.Rank-1)*w, int(c.Suit)*h
	return a.Cards.SubImage(image.Rect(x, y, x+w, y+h)).(*image.RGBA)
}

// ChipImage returns the sprite for chip index i.
func (a *Assets) ChipImage(i int) *image.RGBA {
	s := a.Theme.ChipSize
	n := a.Chips.Rect.Dx() / s
	if n == 0 {
		return a.Chips
	}
	i %= n
	return a.Chips.SubImage(image.Rect(i*s, 0, i*s+s, s)).(*image.RGBA)
}

// ChipFor picks the chip sprite index that best represents an amount.
func (a *Assets) ChipFor(amount int64) int {
	idx := 0
	for i, v := range a.Theme.ChipValues {
		if amount >= v {
			idx = i
		}
	}
	return idx
}

const assetReadme = `THE HOUSE ALWAYS WINS - replaceable assets
==========================================

Every file in this folder can be replaced with your own. Keep the same file
name and (for sprite sheets) the same layout. Delete a file to get the default
back the next time the game starts.

Images (PNG, transparency supported). The game renders at 640x360 and scales
up by whole pixels, so draw at 1x.

  table.png             640x360 background (felt, rail, floor)
  cards.png             card faces: 13 columns (A,2..10,J,Q,K) x 4 rows
                        (spades, hearts, diamonds, clubs). Each card is
                        card_width x card_height from theme.json (24x32).
  card_back.png         one card, same size as a face
  chips.png             a row of chips, chip_size square each, one per
                        entry in theme.json "chip_values"
  dealers.png           the dealer, 9 columns x 2 rows of 48x48 cells. Columns
                        are his wealth tiers from the house's lifetime net:
                        destitute, broke, struggling, working stiff,
                        comfortable, rich, loaded, tycoon, emperor. Row 1 is
                        the regular dealer, row 2 the vulgar one (cigar smoke
                        rises from cigar_x, cigar_y in theme.json, except
                        on the two poorest tiers).
  shoe.png              the card shoe (cards fly out of it)
  logo.png              title logo on the login screen
  default_avatar.png    32x32 picture for players without one
  font.png              16 columns x 6 rows of characters, ASCII 32-127,
                        each font_cell_width x font_cell_height. Draw glyphs
                        in white; they are tinted at runtime. Character
                        widths are measured automatically.
  button*.png, field*.png, panel.png
                        9-slice frames: corners of nine_slice_border
                        (panel_border for panel.png) pixels are kept, the
                        edges and middle are stretched.
  icon.png              32x32 window/taskbar icon
  hats.png              unlockable hats, 8 per row, hat_width x hat_height
                        each (32x20). The bottom hat_overlap rows sit on the
                        avatar. Order: party hat, ball cap, beanie, propeller,
                        cowboy, tin foil, sombrero, cone, top hat, fez,
                        pirate, dunce, viking, halo, devil horns, wizard,
                        unicorn, grad cap, golden top hat, tiara, hair on
                        fire, space helmet, crown, laurel wreath
  frames.png            avatar frames in one row, frame_size square (36x36),
                        a ring around the 32x32 picture. Order: plain, felt,
                        silver, gold, ruby, frozen, bone, inferno, diamond,
                        rainbow (the rainbow one is animated in-game)
  checkbox.png          two boxes side by side: unchecked, checked
  arrow.png             marker shown above the hand whose turn it is

Sounds (WAV, PCM): card, chip, click, win, blackjack, lose, push, bust, turn,
refill, shuffle, error (.wav). The dealer's voice blips (one plays per word
he says): talk1-talk4 for the regular dealer, talklow1-talklow4 for the
vulgar one. chat plays for chat messages.

Card reveal: suspense_<drum|rise|heart>_<5..30>.wav build up to the flip
(the number is tenths of a second; the one that fits the wait is picked).
Stings for how a player's card went: rev_bust, rev_safe (11 or under),
rev_meh (12-16), rev_good (17-18), rev_great (19-20), rev_21. For the
dealer's cards, as the players feel: rev_dealer_bust, rev_dealer_strong
(20-21), rev_dealer_ok, rev_dealer_more (he has to draw again).

theme.json            sizes, chip values and text colours (#rrggbb or
                      #rrggbbaa)
`
