package ge

import (
	"golang.org/x/exp/constraints"
)

func FlattenSlice[S ~[]E, E any](slice []S) S {
	count := 0
	for i := range slice {
		count += len(slice[i])
	}
	res := make(S, 0, count)
	for i := range slice {
		res = append(res, slice[i]...)
	}
	return res
}

func Flatten[S ~[]E, E any](slices ...S) S {
	return FlattenSlice(slices)
}

func CastNumericSlice[T, R constraints.Integer | constraints.Float](input []T) []R {
	if len(input) == 0 {
		return nil
	}
	res := make([]R, len(input))
	for i := range input {
		res[i] = R(input[i])
	}
	return res
}
