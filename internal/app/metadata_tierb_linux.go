//go:build linux

package app

import (
	"time"

	"golang.org/x/sys/unix"
)

var statPath = unix.Stat

func defaultCaptureAtime(path string) (time.Time, error) {
	var stat unix.Stat_t
	if err := statPath(path, &stat); err != nil {
		if isMetadataUnsupported(err) {
			return time.Time{}, errMetadataUnsupported
		}
		return time.Time{}, err
	}
	return time.Unix(stat.Atim.Sec, stat.Atim.Nsec), nil
}
