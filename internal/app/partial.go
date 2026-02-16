package app

import "github.com/shpoont/dotfiles-manager/internal/dfmerr"

func markPartial(err error) error {
	dfmError, ok := dfmerr.As(err)
	if !ok {
		return err
	}

	if dfmError.Details == nil {
		dfmError.Details = map[string]any{}
	}
	dfmError.Details["partial"] = true
	return dfmError
}
