package errutil

import (
	"context"
	"errors"
)

func IgnoreContextCanceledError(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func IgnoreErrors(err error, predicates ...func(err error) bool) error {
	for _, fn := range predicates {
		if fn(err) {
			return nil
		}
	}
	return err
}
