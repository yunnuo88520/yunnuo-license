package service

import (
	"context"
	"os"
	"time"
)

type ReadinessStatus struct {
	DiskFreeBytes uint64 `json:"disk_free_bytes"`
	Ready         bool   `json:"ready"`
	Database      bool   `json:"database"`
	Storage       bool   `json:"storage"`
	Installed     bool   `json:"installed"`
}

func (s *Service) Readiness(ctx context.Context) ReadinessStatus {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out := ReadinessStatus{}
	st := s.currentStore()
	if st == nil {
		return out
	}
	if st.CheckReadiness(ctx) != nil {
		return out
	}
	out.Database = true
	count, err := st.CountAdminUsers(ctx)
	if err != nil {
		return out
	}
	out.Installed = count > 0
	if s.releaseDir != "" {
		if os.MkdirAll(s.releaseDir, 0700) != nil {
			return out
		}
		free, err := availableDiskBytes(s.releaseDir)
		if err != nil {
			return out
		}
		out.DiskFreeBytes = free
		if free < s.resourceLimits.MinFreeBytes {
			return out
		}
		f, err := os.CreateTemp(s.releaseDir, ".ready-*")
		if err != nil {
			return out
		}
		name := f.Name()
		_, err = f.Write([]byte("ready"))
		closeErr := f.Close()
		os.Remove(name)
		if err != nil || closeErr != nil {
			return out
		}
	}
	out.Storage = true
	out.Ready = true
	return out
}
