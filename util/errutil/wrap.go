package errutil

func WrapError(err error, message string) error {
	return wrappedError{err, message}
}

type wrappedError struct {
	Err     error
	Message string
}

func (e wrappedError) Unwrap() error { return e.Err }
func (e wrappedError) Error() string { return e.Message }
