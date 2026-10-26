//go:build !linux && !darwin && !freebsd

package filelock

import (
	"context"
	"errors"
)

func Acquire(context.Context, string, bool) (func(), error) {
	return nil, errors.New("this backend requires POSIX filesystem locks; use the Linux Docker image on this platform")
}
