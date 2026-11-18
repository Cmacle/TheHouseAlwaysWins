// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build linux && !(amd64 || arm64)

package x11

import "errors"

func shmCreate(int) (int, []byte, error) { return 0, nil, errors.New("no shared memory") }
func shmDetach([]byte)                   {}
func shmRemove(int)                      {}
