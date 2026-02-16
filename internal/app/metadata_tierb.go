package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

var (
	captureAtimePath = defaultCaptureAtime
	copyXattrsPath   = defaultCopyXattrs
	copyACLPath      = defaultCopyACL
)

var errMetadataUnsupported = errors.New("metadata unsupported")

func applyTierBMetadata(sourcePath, targetPath string, sourceInfo os.FileInfo) error {
	atime, atimeErr := captureAtimePath(sourcePath)
	if atimeErr == nil {
		if err := chtimesPath(targetPath, atime, sourceInfo.ModTime()); err != nil {
			if !isMetadataUnsupported(err) {
				return wrapMetadataApplyError(targetPath, "atime", err)
			}
		}
	} else if !isMetadataUnsupported(atimeErr) {
		return wrapMetadataApplyError(sourcePath, "atime", atimeErr)
	}

	if err := copyXattrsPath(sourcePath, targetPath); err != nil {
		if !isMetadataUnsupported(err) {
			return wrapMetadataApplyError(targetPath, "xattr", err)
		}
	}

	if err := copyACLPath(sourcePath, targetPath); err != nil {
		if !isMetadataUnsupported(err) {
			return wrapMetadataApplyError(targetPath, "acl", err)
		}
	}

	return nil
}

func wrapMetadataApplyError(path string, metadata string, cause error) error {
	return dfmerr.Wrap(
		dfmerr.CodeMetadataApply,
		fmt.Sprintf("Failed to apply metadata: %s", path),
		map[string]any{"path": path, "metadata": metadata},
		cause,
	)
}

func isMetadataUnsupported(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errMetadataUnsupported) {
		return true
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == syscall.ENOTSUP || errno == syscall.EOPNOTSUPP || errno == syscall.ENOSYS {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not supported") || strings.Contains(msg, "operation unsupported")
}
