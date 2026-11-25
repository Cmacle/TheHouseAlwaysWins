// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build windows

// Package win32 is the Windows platform layer for the client: a plain Win32
// window that shows the 640x360 frame buffer scaled by whole pixels, plus
// input, sound (waveOut, so sounds overlap), the open-file dialog and the clipboard. It uses
// only syscall, so the client builds without cgo.
package win32

import (
	"image"
	"runtime"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
)

func init() { runtime.LockOSThread() }

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pPeekMessageW       = user32.NewProc("PeekMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pShowWindow         = user32.NewProc("ShowWindow")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")
	pBeginPaint         = user32.NewProc("BeginPaint")
	pEndPaint           = user32.NewProc("EndPaint")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pAdjustWindowRect   = user32.NewProc("AdjustWindowRect")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pSetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetKeyState        = user32.NewProc("GetKeyState")
	pOpenClipboard      = user32.NewProc("OpenClipboard")
	pCloseClipboard     = user32.NewProc("CloseClipboard")
	pGetClipboardData   = user32.NewProc("GetClipboardData")
	pMonitorFromWindow  = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW    = user32.NewProc("GetMonitorInfoW")
	pGetCursorPos       = user32.NewProc("GetCursorPos")
	pScreenToClient     = user32.NewProc("ScreenToClient")
	pSetCapture         = user32.NewProc("SetCapture")
	pReleaseCapture     = user32.NewProc("ReleaseCapture")
	pMessageBoxW        = user32.NewProc("MessageBoxW")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pCreateIconFromRes  = user32.NewProc("CreateIconFromResourceEx")
	pGetForegroundWnd   = user32.NewProc("GetForegroundWindow")

	pStretchDIBits     = gdi32.NewProc("StretchDIBits")
	pSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
	pPatBlt            = gdi32.NewProc("PatBlt")
	pGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	pSetDIBitsToDevice = gdi32.NewProc("SetDIBitsToDevice")
	pSetBrushOrgEx     = gdi32.NewProc("SetBrushOrgEx")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")

	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")

	pPlaySoundW      = winmm.NewProc("PlaySoundW")
	pTimeBeginPeriod = winmm.NewProc("timeBeginPeriod")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsPopup            = 0x80000000
	cwUseDefault       = 0x80000000

	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmKillFocus     = 0x0008
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmQuit          = 0x0012
	wmEraseBkgnd    = 0x0014
	wmGetMinMaxInfo = 0x0024
	wmSetIcon       = 0x0080
	wmKeyDown       = 0x0100
	wmChar          = 0x0102
	wmSysKeyDown    = 0x0104
	wmSysChar       = 0x0106
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmRButtonDown   = 0x0204
	wmMouseWheel    = 0x020A

	pmRemove      = 1
	swShow        = 5
	idcArrow      = 32512
	smCxScreen    = 0
	smCyScreen    = 1
	dibRGBColors  = 0
	srcCopy       = 0x00CC0020
	blackness     = 0x00000042
	colorOnColor  = 3
	halftone      = 4
	swpNoZOrder   = 0x0004
	swpFrameChgd  = 0x0020
	swpNoOwnerZ   = 0x0200
	monitorNear   = 2
	cfUnicodeText = 13

	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndMemory    = 0x0004

	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
	ofnExplorer      = 0x00080000
)

var gwlStyle = ^uintptr(15) // -16

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	RcPaint   rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type minMaxInfo struct {
	Reserved     point
	MaxSize      point
	MaxPosition  point
	MinTrackSize point
	MaxTrackSize point
}

type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

type window struct {
	hwnd   uintptr
	bmi    bitmapInfo
	bgra   []byte
	lastFB *image.RGBA
	ox, oy int
	dw, dh int
	lw, lh int // canvas size for this window
	k      int // whole-pixel prescale before the final smooth stretch
	in     client.Input
	quit   bool
	full   bool
	saved  rect
	cfg    *client.Config
	minW   int32
	minH   int32
	frames int
	pk     int  // prescale the bgra buffer was built with
	basic  bool // smooth stretching failed here; use plain stretching
	direct bool // stretching failed entirely; draw unscaled
	failed bool // drawing failure already logged
}

var win *window

func utf16p(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func loword(v uintptr) int32 { return int32(int16(v & 0xffff)) }
func hiword(v uintptr) int32 { return int32(int16((v >> 16) & 0xffff)) }

func keyDown(vk uintptr) bool {
	r, _, _ := pGetKeyState.Call(vk)
	return r&0x8000 != 0
}

// MessageBox shows a simple message dialog.
func MessageBox(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16p(text))), uintptr(unsafe.Pointer(utf16p(title))), 0x10)
}

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	w := win
	if w == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	switch m {
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		w.quit = true
		pPostQuitMessage.Call(0)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if w.lastFB != nil {
			w.blit(ps.Hdc)
		} else {
			cw, ch := w.clientSize()
			pPatBlt.Call(ps.Hdc, 0, 0, uintptr(cw), uintptr(ch), blackness)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmGetMinMaxInfo:
		mmi := *(**minMaxInfo)(unsafe.Pointer(&lp)) // lp points to OS memory
		if w.minW > 0 {
			mmi.MinTrackSize = point{w.minW, w.minH}
		}
		return 0
	case wmKillFocus:
		w.in.Down = false
		w.in.Released = true
		return 0
	case wmLButtonDown:
		w.in.Down = true
		w.in.Pressed = true
		pSetCapture.Call(hwnd)
		return 0
	case wmLButtonUp:
		w.in.Down = false
		w.in.Released = true
		pReleaseCapture.Call()
		return 0
	case wmRButtonDown:
		w.in.RightPressed = true
		return 0
	case wmMouseWheel:
		w.in.Wheel += int(hiword(wp)) / 120
		return 0
	case wmChar:
		if wp >= 32 && wp != 127 {
			w.in.Chars = append(w.in.Chars, rune(wp))
		}
		return 0
	case wmSysChar:
		if wp == 13 { // Alt+Enter, suppress the beep
			return 0
		}
	case wmSysKeyDown:
		if wp == 0x0D {
			w.in.Keys = append(w.in.Keys, client.KeyF11)
			return 0
		}
		if wp == 0x79 { // F10 is a system key
			return 0
		}
	case wmKeyDown:
		var k client.Key
		switch wp {
		case 0x08:
			k = client.KeyBackspace
		case 0x09:
			k = client.KeyTab
		case 0x0D:
			k = client.KeyEnter
		case 0x1B:
			k = client.KeyEscape
		case 0x20:
			k = client.KeySpace
		case 0x21:
			k = client.KeyPageUp
		case 0x22:
			k = client.KeyPageDown
		case 0x23:
			k = client.KeyEnd
		case 0x24:
			k = client.KeyHome
		case 0x25:
			k = client.KeyLeft
		case 0x26:
			k = client.KeyUp
		case 0x27:
			k = client.KeyRight
		case 0x28:
			k = client.KeyDown
		case 0x2E:
			k = client.KeyDelete
		case 0x70:
			k = client.KeyF1
		case 0x7A:
			k = client.KeyF11
		case 0x56: // V
			if keyDown(0x11) {
				k = client.KeyPaste
			}
		}
		if k != client.KeyNone {
			w.in.Keys = append(w.in.Keys, k)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func (w *window) clientSize() (int, int) {
	var r rect
	pGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// layout picks the canvas size for the window's shape and where it goes.
// The canvas is scaled up by whole pixels first and then smoothly to the
// exact window size, so it fills the window and still looks sharp.
func (w *window) layout() {
	cw, ch := w.clientSize()
	w.lw, w.lh = client.ChooseSize(cw, ch)
	s := min(float64(cw)/float64(w.lw), float64(ch)/float64(w.lh))
	w.dw = max(1, int(float64(w.lw)*s+0.5))
	w.dh = max(1, int(float64(w.lh)*s+0.5))
	// a minimized window is 0x0: keep the sizes at least 1 so nothing
	// divides by zero
	w.dw = max(1, min(w.dw, cw))
	w.dh = max(1, min(w.dh, ch))
	w.k = max(1, int(s))
	w.ox, w.oy = (cw-w.dw)/2, (ch-w.dh)/2
}

func (w *window) blit(hdc uintptr) {
	if w.lastFB == nil || len(w.bgra) == 0 {
		return
	}
	cw, ch := w.clientSize()
	pk := max(1, w.pk)
	sw, sh := w.lastFB.Rect.Dx()*pk, w.lastFB.Rect.Dy()*pk
	if len(w.bgra) < sw*sh*4 {
		return
	}
	ok := false
	if !w.direct {
		mode := uintptr(colorOnColor)
		if !w.basic && (sw != w.dw || sh != w.dh) {
			mode = halftone
		}
		pSetStretchBltMode.Call(hdc, mode)
		if mode == halftone {
			pSetBrushOrgEx.Call(hdc, 0, 0, 0)
		}
		r, _, err := pStretchDIBits.Call(hdc,
			uintptr(w.ox), uintptr(w.oy), uintptr(w.dw), uintptr(w.dh),
			0, 0, uintptr(sw), uintptr(sh),
			uintptr(unsafe.Pointer(&w.bgra[0])), uintptr(unsafe.Pointer(&w.bmi)), dibRGBColors, srcCopy)
		ok = r != 0 && int32(r) != -1
		if !ok {
			if mode == halftone {
				Logf("smooth stretch failed (%v), %dx%d -> %dx%d; switching to plain stretch", err, sw, sh, w.dw, w.dh)
				w.basic = true
			} else {
				Logf("stretch failed (%v), %dx%d -> %dx%d; drawing unscaled", err, sw, sh, w.dw, w.dh)
				w.direct = true
			}
			w.blit(hdc)
			return
		}
	} else {
		// last resort: copy the buffer 1:1, centred
		x, y := max(0, (cw-sw)/2), max(0, (ch-sh)/2)
		r, _, err := pSetDIBitsToDevice.Call(hdc, uintptr(x), uintptr(y), uintptr(sw), uintptr(sh),
			0, 0, 0, uintptr(sh),
			uintptr(unsafe.Pointer(&w.bgra[0])), uintptr(unsafe.Pointer(&w.bmi)), dibRGBColors)
		if r == 0 && !w.failed {
			w.failed = true
			Logf("unscaled draw failed too (%v)", err)
		}
		return
	}
	if w.oy > 0 {
		pPatBlt.Call(hdc, 0, 0, uintptr(cw), uintptr(w.oy), blackness)
		pPatBlt.Call(hdc, 0, uintptr(w.oy+w.dh), uintptr(cw), uintptr(ch-w.oy-w.dh), blackness)
	}
	if w.ox > 0 {
		pPatBlt.Call(hdc, 0, uintptr(w.oy), uintptr(w.ox), uintptr(w.dh), blackness)
		pPatBlt.Call(hdc, uintptr(w.ox+w.dw), uintptr(w.oy), uintptr(cw-w.ox-w.dw), uintptr(w.dh), blackness)
	}
}

func (w *window) present(fb *image.RGBA) {
	w.lastFB = fb
	fw, fh, k := fb.Rect.Dx(), fb.Rect.Dy(), max(1, w.k)
	if w.direct {
		k = 1
	}
	w.pk = k
	sw, sh := fw*k, fh*k
	if len(w.bgra) != sw*sh*4 {
		w.bgra = make([]byte, sw*sh*4)
	}
	w.bmi.Header.Width, w.bmi.Header.Height = int32(sw), -int32(sh)
	p, b := fb.Pix, w.bgra
	row := sw * 4
	for y := 0; y < fh; y++ {
		src := p[y*fb.Stride : y*fb.Stride+fw*4]
		dst := b[y*k*row : y*k*row+row]
		o := 0
		for x := 0; x < fw*4; x += 4 {
			r, g, bl := src[x], src[x+1], src[x+2]
			for j := 0; j < k; j++ {
				dst[o], dst[o+1], dst[o+2], dst[o+3] = bl, g, r, 255
				o += 4
			}
		}
		for j := 1; j < k; j++ {
			copy(b[(y*k+j)*row:(y*k+j+1)*row], dst)
		}
	}
	hdc, _, _ := pGetDC.Call(w.hwnd)
	w.blit(hdc)
	pReleaseDC.Call(w.hwnd, hdc)
}

func (w *window) toggleFullscreen() {
	if !w.full {
		pGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&w.saved)))
		mon, _, _ := pMonitorFromWindow.Call(w.hwnd, monitorNear)
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		pSetWindowLongPtrW.Call(w.hwnd, gwlStyle, wsPopup|wsVisible)
		r := mi.Monitor
		pSetWindowPos.Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpFrameChgd|swpNoOwnerZ)
		w.full = true
	} else {
		pSetWindowLongPtrW.Call(w.hwnd, gwlStyle, wsOverlappedWindow|wsVisible)
		r := w.saved
		pSetWindowPos.Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpFrameChgd|swpNoOwnerZ|swpNoZOrder)
		w.full = false
	}
	w.cfg.Fullscreen = w.full
	w.cfg.Save()
}

// ---------------------------------------------------------------- platform

type platform struct{ w *window }

func (p platform) PlaySound(_ string, wav []byte, _ float64) {
	if len(wav) == 0 {
		return
	}
	if playWave(wav) {
		return
	}
	pPlaySoundW.Call(uintptr(unsafe.Pointer(&wav[0])), 0, sndMemory|sndAsync|sndNoDefault)
}

func (p platform) OpenImageFile() (string, bool) {
	filter := utf16.Encode([]rune("Pictures (*.png;*.jpg;*.jpeg;*.gif)\x00*.png;*.jpg;*.jpeg;*.gif\x00All files\x00*.*\x00\x00"))
	buf := make([]uint16, 4096)
	title := utf16p("Choose a profile picture")
	ofn := openFileName{
		Owner:   p.w.hwnd,
		Filter:  &filter[0],
		File:    &buf[0],
		MaxFile: uint32(len(buf)),
		Title:   title,
		Flags:   ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir | ofnExplorer,
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	p.w.in = client.Input{} // the dialog swallowed our mouse-up
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func (p platform) ClipboardText() string {
	if r, _, _ := pOpenClipboard.Call(p.w.hwnd); r == 0 {
		return ""
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	text := *(**[4096]uint16)(unsafe.Pointer(&ptr)) // OS-owned clipboard memory
	var out []uint16
	for _, c := range text {
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return string(utf16.Decode(out))
}

func (p platform) ToggleFullscreen() { p.w.toggleFullscreen() }

func (platform) UpdateAsset() string             { return "TheHouseAlwaysWins.exe" }
func (platform) UpdateDir() string               { return selfupdate.Dir() }
func (platform) InstallUpdate(path string) error { return selfupdate.Replace(path) }
func (p platform) Quit()                         { pDestroyWindow.Call(p.w.hwnd) }

// ---------------------------------------------------------------- run

// Run opens the window and runs the client until it is closed.
func Run(cfg *client.Config, assets *client.Assets, icon []byte) error {
	return RunApp("The House Always Wins", cfg, icon, func(p client.Platform) *client.App { return client.New(p, cfg, assets) })
}

// RunApp opens a window titled title and runs the app made by mk in it
// until the window closes.
func RunApp(title string, cfg *client.Config, icon []byte, mk func(client.Platform) *client.App) error {
	pSetProcessDPIAware.Call()
	pTimeBeginPeriod.Call(1)
	LogSystem()

	inst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	className := utf16p("TheHouseAlwaysWins")
	wc := wndClassEx{
		Style:     0x0003, // CS_HREDRAW | CS_VREDRAW
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  inst,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return err
	}

	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
	scale := cfg.WindowScale
	if scale <= 0 {
		scale = 1
		for s := 2; s <= 8; s++ {
			if client.ScreenW*s <= int(sw)*9/10 && client.ScreenH*s <= int(sh)*85/100 {
				scale = s
			}
		}
	}
	r := rect{0, 0, int32(client.ScreenW * scale), int32(client.ScreenH * scale)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), wsOverlappedWindow, 0)
	ww, wh := r.Right-r.Left, r.Bottom-r.Top
	min := rect{0, 0, 320, 180}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&min)), wsOverlappedWindow, 0)

	w := &window{cfg: cfg, bgra: make([]byte, client.ScreenW*client.ScreenH*4), minW: min.Right - min.Left, minH: min.Bottom - min.Top, k: 1}
	w.bmi.Header = bitmapInfoHeader{Width: int32(client.ScreenW), Height: -int32(client.ScreenH), Planes: 1, BitCount: 32}
	w.bmi.Header.Size = uint32(unsafe.Sizeof(w.bmi.Header))
	win = w

	x := (int32(sw) - ww) / 2
	y := (int32(sh) - wh) / 2
	hwnd, _, err := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16p(title))),
		wsOverlappedWindow|wsVisible,
		uintptr(x), uintptr(y), uintptr(ww), uintptr(wh),
		0, 0, inst, 0)
	if hwnd == 0 {
		Logf("CreateWindowEx failed: %v", err)
		return err
	}
	w.hwnd = hwnd
	Logf("window %dx%d at %d,%d (scale %d)", ww, wh, x, y, scale)
	if len(icon) > 0 {
		for _, size := range []uintptr{0, 1} { // ICON_SMALL, ICON_BIG
			px := uintptr(16)
			if size == 1 {
				px = 32
			}
			h, _, _ := pCreateIconFromRes.Call(uintptr(unsafe.Pointer(&icon[0])), uintptr(len(icon)), 1, 0x00030000, px, px, 0)
			if h != 0 {
				pSendMessageW.Call(hwnd, wmSetIcon, size, h)
			}
		}
	}
	pShowWindow.Call(hwnd, swShow)
	if cfg.Fullscreen {
		w.toggleFullscreen()
	}

	setStage("starting the game")
	app := mk(platform{w})
	defer app.Close()
	heartbeat.Store(time.Now().UnixNano())
	go watchdog()

	const frameTime = time.Second / 60
	next := time.Now()
	var m msg
	for !w.quit {
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			if m.Message == wmQuit {
				w.quit = true
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if w.quit {
			break
		}
		// mouse position in virtual pixels
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		w.layout()
		app.SetSize(w.lw, w.lh)
		in := w.in
		in.MX = (int(pt.X) - w.ox) * w.lw / max(1, w.dw)
		in.MY = (int(pt.Y) - w.oy) * w.lh / max(1, w.dh)
		fg, _, _ := pGetForegroundWnd.Call()
		in.MouseInside = fg == hwnd && in.MX >= 0 && in.MY >= 0 && in.MX < w.lw && in.MY < w.lh
		in.Shift = keyDown(0x10)
		in.Ctrl = keyDown(0x11)

		setStage("drawing a frame")
		fb := app.Frame(in)
		setStage("showing a frame")
		w.present(fb)
		setStage("sounds")
		if w.frames++; w.frames%30 == 0 {
			reapVoices()
		}
		if w.frames == 1 {
			Logf("first frame shown, canvas %dx%d drawn at %dx%d (prescale %d)", w.lw, w.lh, w.dw, w.dh, w.pk)
		}
		heartbeat.Store(time.Now().UnixNano())
		setStage("window messages")

		// reset per-frame input
		w.in.Pressed, w.in.Released, w.in.RightPressed = false, false, false
		w.in.Wheel = 0
		w.in.Chars = w.in.Chars[:0]
		w.in.Keys = w.in.Keys[:0]

		next = next.Add(frameTime)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		} else if d < -100*time.Millisecond {
			next = time.Now()
		}
	}
	Logf("window closed after %d frames", w.frames)
	stopAllSounds()
	cfg.Save()
	return nil
}
