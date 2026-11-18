//go:build linux && (amd64 || arm64)

package x11

import (
	"syscall"
	"unsafe"
)

// System V shared memory for MIT-SHM.

func shmCreate(size int) (int, []byte, error) {
	id, _, e := syscall.Syscall(syscall.SYS_SHMGET, 0, uintptr(size), 0o1000|0o600) // IPC_PRIVATE, IPC_CREAT|0600
	if e != 0 {
		return 0, nil, e
	}
	addr, _, e := syscall.Syscall(syscall.SYS_SHMAT, id, 0, 0)
	if e != 0 {
		shmRemove(int(id))
		return 0, nil, e
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return int(id), unsafe.Slice((*byte)(p), size), nil
}

func shmDetach(mem []byte) {
	syscall.Syscall(syscall.SYS_SHMDT, uintptr(unsafe.Pointer(&mem[0])), 0, 0)
}

func shmRemove(id int) {
	syscall.Syscall(syscall.SYS_SHMCTL, uintptr(id), 0, 0) // IPC_RMID
}
