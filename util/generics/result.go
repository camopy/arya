package ge

import "errors"

var errValueSet = errors.New("marker: value is set")

type Result[T any] struct {
	value T
	err   error
}

func NewResult[T any](value T, err error) Result[T] {
	var r Result[T]
	r.Set(value, err)
	return r
}

func (r *Result[T]) Reset() {
	*r = Result[T]{}
}

func (r *Result[T]) IsUnset() bool {
	return r.err == nil
}

func (r *Result[T]) IsSet() bool {
	return r.err != nil
}

func (r *Result[T]) Set(value T, err error) {
	if err == nil {
		err = errValueSet
	}
	r.value, r.err = value, err
}

func (r *Result[T]) Get() (T, error) {
	//goland:noinspection GoDirectComparisonOfErrors
	if r.err == errValueSet { //nolint:errorlint // valid, error set internally
		return r.value, nil
	}
	return r.value, r.err
}

func (r *Result[T]) Ok() bool {
	//goland:noinspection GoDirectComparisonOfErrors
	return r.err == errValueSet //nolint:errorlint // valid, error set internally
}

func (r *Result[T]) Err() error {
	//goland:noinspection GoDirectComparisonOfErrors
	if r.err == errValueSet { //nolint:errorlint // valid, error set internally
		return nil
	}
	return r.err
}

func (r *Result[T]) Value() T {
	return r.value
}
