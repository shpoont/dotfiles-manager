//go:build linux || darwin

package app

import (
	"bytes"
	"fmt"
	"os/exec"

	"golang.org/x/sys/unix"
)

var (
	listXattrNamesPath = listXattrNames
	readXattrPath      = readXattr
	setXattrPath       = unix.Setxattr
	lookPathPath       = exec.LookPath
	readACLPath        = readACLFromSource
	writeACLPath       = writeACLToTarget
)

func defaultCopyXattrs(sourcePath, targetPath string) error {
	names, err := listXattrNamesPath(sourcePath)
	if err != nil {
		if isMetadataUnsupported(err) {
			return errMetadataUnsupported
		}
		return err
	}

	for _, name := range names {
		value, err := readXattrPath(sourcePath, name)
		if err != nil {
			if isMetadataUnsupported(err) {
				return errMetadataUnsupported
			}
			return err
		}
		if err := setXattrPath(targetPath, name, value, 0); err != nil {
			if isMetadataUnsupported(err) {
				return errMetadataUnsupported
			}
			return err
		}
	}

	return nil
}

func listXattrNames(path string) ([]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return []string{}, nil
	}

	buf := make([]byte, size)
	n, err := unix.Listxattr(path, buf)
	if err != nil {
		return nil, err
	}

	parts := bytes.Split(buf[:n], []byte{0})
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		names = append(names, string(part))
	}

	return names, nil
}

func readXattr(path, name string) ([]byte, error) {
	size, err := unix.Getxattr(path, name, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, size)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func defaultCopyACL(sourcePath, targetPath string) error {
	getfaclPath, err := lookPathPath("getfacl")
	if err != nil {
		return errMetadataUnsupported
	}
	setfaclPath, err := lookPathPath("setfacl")
	if err != nil {
		return errMetadataUnsupported
	}

	aclPayload, err := readACLPath(getfaclPath, sourcePath)
	if err != nil {
		if isMetadataUnsupported(err) {
			return errMetadataUnsupported
		}
		return fmt.Errorf("getfacl failed: %w", err)
	}

	if err := writeACLPath(setfaclPath, targetPath, aclPayload); err != nil {
		if isMetadataUnsupported(err) {
			return errMetadataUnsupported
		}
		return fmt.Errorf("setfacl failed: %w", err)
	}

	return nil
}

func readACLFromSource(getfaclPath, sourcePath string) ([]byte, error) {
	output, err := exec.Command(getfaclPath, "-p", sourcePath).CombinedOutput()
	if err != nil {
		if isMetadataUnsupported(fmt.Errorf("%s", output)) {
			return nil, errMetadataUnsupported
		}
		return nil, err
	}
	return output, nil
}

func writeACLToTarget(setfaclPath, targetPath string, payload []byte) error {
	cmd := exec.Command(setfaclPath, "--set-file=-", targetPath)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isMetadataUnsupported(fmt.Errorf("%s", output)) {
			return errMetadataUnsupported
		}
		return err
	}
	return nil
}
