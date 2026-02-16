//go:build !linux && !darwin

package app

import "time"

func defaultCaptureAtime(string) (time.Time, error) {
	return time.Time{}, errMetadataUnsupported
}

func defaultCopyXattrs(string, string) error {
	return errMetadataUnsupported
}

func defaultCopyACL(string, string) error {
	return errMetadataUnsupported
}
