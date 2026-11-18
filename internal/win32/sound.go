// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build windows

package win32

import (
	"encoding/binary"
	"sync"
	"unsafe"
)

// Sounds play through waveOut, one device handle per sound, so several can
// overlap (card flicks, chips and the dealer's voice blips at the same time).
// Windows mixes them. Falls back to PlaySound if waveOut is unavailable.

var (
	pWaveOutOpen      = winmm.NewProc("waveOutOpen")
	pWaveOutPrepare   = winmm.NewProc("waveOutPrepareHeader")
	pWaveOutWrite     = winmm.NewProc("waveOutWrite")
	pWaveOutUnprepare = winmm.NewProc("waveOutUnprepareHeader")
	pWaveOutClose     = winmm.NewProc("waveOutClose")
	pWaveOutReset     = winmm.NewProc("waveOutReset")
)

const (
	waveMapper  = 0xFFFFFFFF
	whdrDone    = 0x00000001
	maxVoices   = 12
	callbackNul = 0
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

type waveHdr struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

type voice struct {
	h    uintptr
	hdr  *waveHdr
	data []byte // kept alive while playing
	fmt  *waveFormatEx
}

var (
	voiceMu sync.Mutex
	voices  []*voice
)

// parseWav finds the format and sample data of a PCM WAV file.
func parseWav(b []byte) (*waveFormatEx, []byte, bool) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, nil, false
	}
	var f *waveFormatEx
	var data []byte
	for p := 12; p+8 <= len(b); {
		id := string(b[p : p+4])
		n := int(binary.LittleEndian.Uint32(b[p+4 : p+8]))
		body := p + 8
		if n < 0 || body+n > len(b) {
			n = len(b) - body
		}
		switch id {
		case "fmt ":
			if n >= 16 {
				f = &waveFormatEx{
					FormatTag:      binary.LittleEndian.Uint16(b[body:]),
					Channels:       binary.LittleEndian.Uint16(b[body+2:]),
					SamplesPerSec:  binary.LittleEndian.Uint32(b[body+4:]),
					AvgBytesPerSec: binary.LittleEndian.Uint32(b[body+8:]),
					BlockAlign:     binary.LittleEndian.Uint16(b[body+12:]),
					BitsPerSample:  binary.LittleEndian.Uint16(b[body+14:]),
				}
			}
		case "data":
			data = b[body : body+n]
		}
		p = body + n + n%2
	}
	if f == nil || data == nil || f.FormatTag != 1 || len(data) == 0 {
		return nil, nil, false
	}
	return f, data, true
}

// reapVoices closes devices whose sound has finished.
func reapVoices() {
	voiceMu.Lock()
	defer voiceMu.Unlock()
	live := voices[:0]
	for _, v := range voices {
		if v.hdr.Flags&whdrDone != 0 {
			pWaveOutUnprepare.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr))
			pWaveOutClose.Call(v.h)
			continue
		}
		live = append(live, v)
	}
	for i := len(live); i < len(voices); i++ {
		voices[i] = nil
	}
	voices = live
}

// playWave starts a sound on its own waveOut device. Returns false if the
// sound couldn't be played this way.
func playWave(wav []byte) bool {
	f, data, ok := parseWav(wav)
	if !ok {
		return false
	}
	reapVoices()
	voiceMu.Lock()
	defer voiceMu.Unlock()
	if len(voices) >= maxVoices {
		// too many at once: cut the oldest
		v := voices[0]
		pWaveOutReset.Call(v.h)
		pWaveOutUnprepare.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr))
		pWaveOutClose.Call(v.h)
		voices = voices[1:]
	}
	v := &voice{fmt: f, data: data}
	if r, _, _ := pWaveOutOpen.Call(uintptr(unsafe.Pointer(&v.h)), waveMapper, uintptr(unsafe.Pointer(f)), 0, 0, callbackNul); r != 0 {
		return false
	}
	v.hdr = &waveHdr{Data: uintptr(unsafe.Pointer(&data[0])), BufferLength: uint32(len(data))}
	if r, _, _ := pWaveOutPrepare.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr)); r != 0 {
		pWaveOutClose.Call(v.h)
		return false
	}
	if r, _, _ := pWaveOutWrite.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr)); r != 0 {
		pWaveOutUnprepare.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr))
		pWaveOutClose.Call(v.h)
		return false
	}
	voices = append(voices, v)
	return true
}

// stopAllSounds closes every device (on exit).
func stopAllSounds() {
	voiceMu.Lock()
	defer voiceMu.Unlock()
	for _, v := range voices {
		pWaveOutReset.Call(v.h)
		pWaveOutUnprepare.Call(v.h, uintptr(unsafe.Pointer(v.hdr)), unsafe.Sizeof(*v.hdr))
		pWaveOutClose.Call(v.h)
	}
	voices = nil
}
