package dfmerr

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeConfigRequired         Code = "DFM_CONFIG_REQUIRED"
	CodeConfigNotFound         Code = "DFM_CONFIG_NOT_FOUND"
	CodeConfigNotFile          Code = "DFM_CONFIG_NOT_FILE"
	CodeConfigParse            Code = "DFM_CONFIG_PARSE"
	CodeConfigSchemaUnknownKey Code = "DFM_CONFIG_SCHEMA_UNKNOWN_KEY"
	CodeConfigSchemaType       Code = "DFM_CONFIG_SCHEMA_TYPE"
	CodeConfigSchemaRequired   Code = "DFM_CONFIG_SCHEMA_REQUIRED"
	CodeConfigPathNotRelative  Code = "DFM_CONFIG_PATH_NOT_RELATIVE"
	CodeConfigPathEscape       Code = "DFM_CONFIG_PATH_ESCAPE"

	CodeFlagUnsupported  Code = "DFM_FLAG_UNSUPPORTED"
	CodeFlagInvalidValue Code = "DFM_FLAG_INVALID_VALUE"
	CodeScopeNoMatch     Code = "DFM_SCOPE_NO_MATCH"
	CodeScopeInvalidPath Code = "DFM_SCOPE_INVALID_PATH"

	CodeIORead        Code = "DFM_IO_READ"
	CodeIOWrite       Code = "DFM_IO_WRITE"
	CodeIORemove      Code = "DFM_IO_REMOVE"
	CodeTypeReplace   Code = "DFM_TYPE_REPLACE"
	CodeMetadataApply Code = "DFM_METADATA_APPLY"
)

type Error struct {
	Code    Code
	Message string
	Details map[string]any
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code Code, message string, details map[string]any) *Error {
	return &Error{Code: code, Message: message, Details: details}
}

func Wrap(code Code, message string, details map[string]any, cause error) *Error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}

func WithDetails(err error, details map[string]any) error {
	var dfmErr *Error
	if !errors.As(err, &dfmErr) {
		return err
	}
	dfmErr.Details = details
	return dfmErr
}

func As(err error) (*Error, bool) {
	var dfmErr *Error
	if errors.As(err, &dfmErr) {
		return dfmErr, true
	}
	return nil, false
}

func MustCode(err error) Code {
	if err == nil {
		return ""
	}
	var dfmErr *Error
	if errors.As(err, &dfmErr) {
		return dfmErr.Code
	}
	return ""
}

func InvalidFlagValue(flag, value, expected string) error {
	message := fmt.Sprintf("Invalid value for %s: %s (expected: %s)", flag, value, expected)
	return New(CodeFlagInvalidValue, message, map[string]any{
		"flag":     flag,
		"value":    value,
		"expected": expected,
	})
}
