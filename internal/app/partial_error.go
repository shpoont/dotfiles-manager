package app

import "errors"

type partialCommandError struct {
	err     error
	syncs   []any
	summary map[string]any
}

func (e *partialCommandError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *partialCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func newPartialCommandError(err error, syncs []any, summary map[string]any) error {
	if err == nil {
		return nil
	}
	marked := markPartial(err)
	return &partialCommandError{
		err:     marked,
		syncs:   syncs,
		summary: summary,
	}
}

func asPartialCommandError(err error) (*partialCommandError, bool) {
	var partial *partialCommandError
	if errors.As(err, &partial) {
		return partial, true
	}
	return nil, false
}
