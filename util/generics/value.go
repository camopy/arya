package ge

func Zero[T any]() T {
	var zero T
	return zero
}

func IsZero[T comparable](v T) bool {
	var zero T
	return v == zero
}

func ValueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func NilIfZero[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

func ValueOrDefault[T any](value *T, defaultValue T) T {
	if value == nil {
		return defaultValue
	}
	return *value
}

func DefaultIfZero[T comparable](value T, defaultValue T) T {
	var zero T
	if value == zero {
		return defaultValue
	}
	return value
}

func FirstNonNil[T any](values ...*T) *T {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func FirstNonZero[T comparable](values ...T) T {
	var zero T
	for _, v := range values {
		if v != zero {
			return v
		}
	}
	return zero
}

func NonNilToSlice[T any](values ...*T) []T {
	count := 0
	for _, v := range values {
		if v != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	res := make([]T, 0, count)
	for _, v := range values {
		if v != nil {
			res = append(res, *v)
		}
	}
	return res
}
