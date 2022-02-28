//go:build !windows
// +build !windows

package vlog

import (
	"os"
	"syscall"
)

// create new file with file info's mode,user,group
func chown(name string, info os.FileInfo) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_ = f.Close()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return osChown(name, int(stat.Uid), int(stat.Gid))
	}
	return nil
}
