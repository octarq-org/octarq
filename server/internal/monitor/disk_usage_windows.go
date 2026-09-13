//go:build windows

package monitor

import (
	"syscall"
	"unsafe"
)

func getDiskUsage(path string) (total, free, avail uint64, err error) {
	kernel32, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return 0, 0, 0, err
	}
	proc, err := kernel32.FindProc("GetDiskFreeSpaceExW")
	if err != nil {
		return 0, 0, 0, err
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, err
	}
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64
	r1, _, err := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, 0, err
	}
	return uint64(totalNumberOfBytes), uint64(totalNumberOfFreeBytes), uint64(freeBytesAvailable), nil
}
