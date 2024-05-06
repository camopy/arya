package errutil

import "errors"

func FatalError(err error) error {
	return fatalError{err}
}

func IsFatalError(err error) bool {
	return errors.Is(err, fatalError{})
}

type fatalError struct {
	error
}

func (e fatalError) Unwrap() error { return e.error }
