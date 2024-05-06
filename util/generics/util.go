package ge

func Ptr[T any](v T) *T {
	return &v
}

func Cond[T any](cond bool, trueValue T, falseValue T) T {
	if cond {
		return trueValue
	}
	return falseValue
}
