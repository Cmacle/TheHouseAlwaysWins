// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build windows

package win32

import (
	"errors"
	"syscall"
	"unsafe"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
)

// The remembered password is encrypted with Windows' data protection
// (DPAPI): only the same Windows user on the same PC can decrypt it.

var (
	crypt32             = syscall.NewLazyDLL("crypt32.dll")
	pCryptProtectData   = crypt32.NewProc("CryptProtectData")
	pCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	pLocalFree          = kernel32.NewProc("LocalFree")
	dpapiEntropy        = []byte("TheHouseAlwaysWins saved login")
)

const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	n    uint32
	data *byte
}

func blob(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{n: uint32(len(b)), data: &b[0]}
}

func (b *dataBlob) bytes() []byte {
	if b.data == nil || b.n == 0 {
		return nil
	}
	out := make([]byte, b.n)
	copy(out, unsafe.Slice(b.data, b.n))
	return out
}

func dpapi(proc *syscall.LazyProc, in []byte) ([]byte, error) {
	if proc.Find() != nil {
		return nil, errors.New("data protection isn't available")
	}
	var out dataBlob
	r, _, err := proc.Call(uintptr(unsafe.Pointer(blob(in))), 0, uintptr(unsafe.Pointer(blob(dpapiEntropy))),
		0, 0, cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return out.bytes(), nil
}

func init() {
	client.ProtectSecret = func(b []byte) ([]byte, error) { return dpapi(pCryptProtectData, b) }
	client.UnprotectSecret = func(b []byte) ([]byte, error) { return dpapi(pCryptUnprotectData, b) }
}
