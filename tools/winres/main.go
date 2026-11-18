// Command winres writes a Windows resource object (.syso) with the app's
// icon, version information and manifest, which `go build` links into the
// exe. Windows (and antivirus scanners) treat a program with a proper
// publisher, version and manifest much more kindly than an anonymous one.
//
//	go run ./tools/winres -version 1.7.0 -name "The House Always Wins" \
//	    -exe TheHouseAlwaysWins.exe -out cmd/client/rsrc_windows_amd64.syso
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

func main() {
	version := flag.String("version", "0.0.0", "version (1.2.3)")
	name := flag.String("name", "The House Always Wins", "file description")
	exe := flag.String("exe", "TheHouseAlwaysWins.exe", "original file name")
	icon := flag.String("icon", "internal/client/defaultassets/icon.png", "PNG icon")
	gui := flag.Bool("gui", true, "a windowed program (sets the manifest's DPI awareness)")
	out := flag.String("out", "", "output .syso")
	flag.Parse()
	if *out == "" {
		fail(fmt.Errorf("-out is required"))
	}
	v := parseVersion(*version)
	res := []resource{}

	// icon: PNG images in several sizes, plus the group that lists them
	if *icon != "" {
		f, err := os.Open(*icon)
		if err != nil {
			fail(err)
		}
		src, err := png.Decode(f)
		f.Close()
		if err != nil {
			fail(err)
		}
		var group bytes.Buffer
		sizes := []int{16, 24, 32, 48, 64, 128, 256}
		w16(&group, 0, 1, uint16(len(sizes)))
		for i, s := range sizes {
			var b bytes.Buffer
			png.Encode(&b, scale(src, s))
			id := uint16(i + 1)
			res = append(res, resource{typ: 3, id: id, data: b.Bytes()}) // RT_ICON
			dim := byte(s)
			if s >= 256 {
				dim = 0
			}
			group.Write([]byte{dim, dim, 0, 0})
			w16(&group, 1, 32)
			binary.Write(&group, binary.LittleEndian, uint32(b.Len()))
			w16(&group, id)
		}
		res = append(res, resource{typ: 14, id: 1, data: group.Bytes()}) // RT_GROUP_ICON
	}

	// version information
	vs := fmt.Sprintf("%d.%d.%d.0", v[0], v[1], v[2])
	strs := [][2]string{
		{"CompanyName", "Cmacle"},
		{"FileDescription", *name},
		{"FileVersion", vs},
		{"InternalName", strings.TrimSuffix(*exe, ".exe")},
		{"LegalCopyright", "Copyright (c) Cmacle"},
		{"OriginalFilename", *exe},
		{"ProductName", "The House Always Wins"},
		{"ProductVersion", vs},
		{"Comments", "https://github.com/Cmacle/TheHouseAlwaysWins"},
	}
	res = append(res, resource{typ: 16, id: 1, data: versionInfo(v, strs)}) // RT_VERSION

	// manifest
	dpi := ""
	if *gui {
		dpi = `
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>
    </windowsSettings>
  </application>`
	}
	manifest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="Cmacle.%s" version="%s" processorArchitecture="amd64"/>
  <description>%s</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
      <supportedOS Id="{1f676c76-80e1-4239-95bb-83d0f6d0da78}"/>
      <supportedOS Id="{4a2f28e3-53b9-4441-ba9c-d69d4a4a6e38}"/>
      <supportedOS Id="{35138b9a-5d96-4fbd-8e2d-a2440225f93a}"/>
    </application>
  </compatibility>%s
</assembly>
`, strings.TrimSuffix(*exe, ".exe"), vs, *name, dpi)
	res = append(res, resource{typ: 24, id: 1, data: []byte(manifest)}) // RT_MANIFEST

	if err := os.WriteFile(*out, coff(res), 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "winres:", err)
	os.Exit(1)
}

type resource struct {
	typ, id uint16
	data    []byte
}

func w16(b *bytes.Buffer, vs ...uint16) {
	for _, v := range vs {
		binary.Write(b, binary.LittleEndian, v)
	}
}

func parseVersion(s string) [3]uint16 {
	var v [3]uint16
	for i, p := range strings.SplitN(strings.TrimPrefix(s, "v"), ".", 3) {
		if j := strings.IndexAny(p, "-+ "); j >= 0 {
			p = p[:j]
		}
		n, _ := strconv.Atoi(p)
		v[i] = uint16(n)
	}
	return v
}

// scale resizes with nearest neighbour (it's pixel art).
func scale(src image.Image, s int) *image.NRGBA {
	b := src.Bounds()
	in := image.NewNRGBA(b)
	draw.Draw(in, b, src, b.Min, draw.Src)
	out := image.NewNRGBA(image.Rect(0, 0, s, s))
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			c := in.NRGBAAt(b.Min.X+x*b.Dx()/s, b.Min.Y+y*b.Dy()/s)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// ------------------------------------------------------------ VERSIONINFO

func utf16z(s string) []byte {
	var b bytes.Buffer
	for _, r := range utf16.Encode([]rune(s)) {
		w16(&b, r)
	}
	w16(&b, 0)
	return b.Bytes()
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// block builds one VERSIONINFO node: length, value length, type, key,
// value, children, each part aligned to 4 bytes.
func block(key string, value []byte, valueLen, typ uint16, children ...[]byte) []byte {
	var b bytes.Buffer
	w16(&b, 0, valueLen, typ)
	b.Write(utf16z(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

func versionInfo(v [3]uint16, strs [][2]string) []byte {
	var fixed bytes.Buffer
	ms, ls := uint32(v[0])<<16|uint32(v[1]), uint32(v[2])<<16
	for _, x := range []uint32{0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x00040004, 1, 0, 0, 0} {
		binary.Write(&fixed, binary.LittleEndian, x)
	}
	var kids [][]byte
	for _, kv := range strs {
		val := utf16z(kv[1])
		kids = append(kids, block(kv[0], val, uint16(len(val)/2), 1))
	}
	table := block("040904B0", nil, 0, 1, kids...)
	sfi := block("StringFileInfo", nil, 0, 1, table)
	var tr bytes.Buffer
	w16(&tr, 0x0409, 0x04B0)
	vfi := block("VarFileInfo", nil, 0, 1, block("Translation", tr.Bytes(), 4, 0))
	return block("VS_VERSION_INFO", fixed.Bytes(), uint16(fixed.Len()), 0, sfi, vfi)
}

// ------------------------------------------------------------ COFF object

// coff lays the resources out as a .rsrc section: a three-level directory
// (type, id, language), the data entries, then the data. Data entries hold
// addresses relative to the image, so each gets a relocation the linker
// fills in.
func coff(res []resource) []byte {
	sort.Slice(res, func(i, j int) bool {
		if res[i].typ != res[j].typ {
			return res[i].typ < res[j].typ
		}
		return res[i].id < res[j].id
	})
	var types []uint16
	byType := map[uint16][]int{}
	for i, r := range res {
		if len(byType[r.typ]) == 0 {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], i)
	}
	dirSize := func(n int) int { return 16 + 8*n }
	// sizes: root, one dir per type, one lang dir per resource, data entries
	off := dirSize(len(types))
	typeDir := map[uint16]int{}
	for _, t := range types {
		typeDir[t] = off
		off += dirSize(len(byType[t]))
	}
	langDir := make([]int, len(res))
	for i := range res {
		langDir[i] = off
		off += dirSize(1)
	}
	entry := make([]int, len(res))
	for i := range res {
		entry[i] = off
		off += 16
	}
	dataAt := make([]int, len(res))
	for i, r := range res {
		off = (off + 7) &^ 7
		dataAt[i] = off
		off += len(r.data)
	}
	sec := make([]byte, (off+7)&^7)
	le := binary.LittleEndian
	dir := func(at, n int) {
		le.PutUint16(sec[at+14:], uint16(n)) // ID entries
	}
	ent := func(at int, id uint32, target uint32) {
		le.PutUint32(sec[at:], id)
		le.PutUint32(sec[at+4:], target)
	}
	dir(0, len(types))
	for k, t := range types {
		ent(16+8*k, uint32(t), 0x80000000|uint32(typeDir[t]))
		dir(typeDir[t], len(byType[t]))
		for j, i := range byType[t] {
			ent(typeDir[t]+16+8*j, uint32(res[i].id), 0x80000000|uint32(langDir[i]))
		}
	}
	var relocs []uint32
	for i, r := range res {
		dir(langDir[i], 1)
		ent(langDir[i]+16, 0x0409, uint32(entry[i])) // data entry (no high bit)
		le.PutUint32(sec[entry[i]:], uint32(dataAt[i]))
		le.PutUint32(sec[entry[i]+4:], uint32(len(r.data)))
		relocs = append(relocs, uint32(entry[i]))
		copy(sec[dataAt[i]:], r.data)
	}

	var b bytes.Buffer
	const fileHdr, secHdr = 20, 40
	rawAt := fileHdr + secHdr
	relAt := rawAt + len(sec)
	symAt := relAt + 10*len(relocs)
	// file header
	w16(&b, 0x8664, 1)
	binary.Write(&b, le, uint32(0))
	binary.Write(&b, le, uint32(symAt))
	binary.Write(&b, le, uint32(1)) // symbols
	w16(&b, 0, 0)
	// section header
	b.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&b, le, uint32(0)) // virtual size
	binary.Write(&b, le, uint32(0)) // virtual address
	binary.Write(&b, le, uint32(len(sec)))
	binary.Write(&b, le, uint32(rawAt))
	binary.Write(&b, le, uint32(relAt))
	binary.Write(&b, le, uint32(0))
	w16(&b, uint16(len(relocs)), 0)
	binary.Write(&b, le, uint32(0x40000040)) // initialized data, readable
	b.Write(sec)
	for _, r := range relocs {
		binary.Write(&b, le, r)         // offset in the section
		binary.Write(&b, le, uint32(0)) // symbol 0: the section
		w16(&b, 3)                      // IMAGE_REL_AMD64_ADDR32NB
	}
	// symbol table: .rsrc, static, section 1
	b.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&b, le, uint32(0))
	w16(&b, 1, 0)
	b.Write([]byte{3, 0})
	binary.Write(&b, le, uint32(4)) // empty string table
	return b.Bytes()
}
