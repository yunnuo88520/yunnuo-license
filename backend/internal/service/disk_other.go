//go:build !linux && !darwin && !freebsd

package service

import "errors"

func availableDiskBytes(string) (uint64, error) {
	return 0, errors.New("disk readiness is unsupported on this platform; use the Linux Docker image")
}
