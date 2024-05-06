package ge

type Comparable[T any] interface {
	Compare(other T) int
}

func CompareMulti(compareResults ...int) int {
	for _, cmp := range compareResults {
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

func CompareSign(reverse bool) int {
	if reverse {
		return -1
	}
	return 1
}

func ReverseCompare[T any](cmp func(T, T) int) func(T, T) int {
	return func(a, b T) int { return cmp(b, a) }
}

func ReverseCompareCond[T any](reverse bool, cmp func(T, T) int) func(T, T) int {
	if reverse {
		return ReverseCompare(cmp)
	}
	return cmp
}
