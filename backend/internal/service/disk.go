//go:build linux || darwin || freebsd

package service

import "syscall"

func availableDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	return stat.Bavail * uint64(stat.Bsize), err
}
