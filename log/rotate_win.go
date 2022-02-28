//go:build windows
// +build windows

package vlog

import "os"

// create new file with file info's mode only, windows is not support user,group
func chown(name string, info os.FileInfo) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err == nil {
		_ = f.Close()
	}
	return err
}
