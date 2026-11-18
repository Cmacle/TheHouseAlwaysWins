// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package main

import "unsafe"

func unsafePtr(p *uint32) unsafe.Pointer { return unsafe.Pointer(p) }
