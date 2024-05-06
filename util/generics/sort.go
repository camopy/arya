package ge

import (
	"cmp"
	"slices"
	"sort"

	"golang.org/x/exp/constraints"
)

func UniqueSorted[S ~[]E, E constraints.Ordered](slice S) S {
	slices.Sort(slice)
	return slices.Compact(slice)
}

func SortDesc[S ~[]E, E constraints.Ordered](slice S) {
	slices.SortFunc(slice, ReverseCompare(cmp.Compare[E]))
}

func SortBy[S ~[]E, E any, C constraints.Ordered](slice S, compareBy func(v E) C) {
	sortBy(slice, false, compareBy)
}

func SortByDesc[T any, C constraints.Ordered](slice []T, compareBy func(v T) C) {
	sortBy(slice, true, compareBy)
}

func sortBy[S ~[]E, E any, C constraints.Ordered](slice S, reverse bool, compareBy func(v E) C) {
	slices.SortFunc(slice, ReverseCompareCond(reverse, func(a, b E) int {
		return cmp.Compare(compareBy(a), compareBy(b))
	}))
}

func SortByComparator[S ~[]E, E any](slice S, cmp func(a, b E) int) {
	slices.SortFunc(slice, cmp)
}

func SortByComparatorPtr[S ~[]E, E any](slice S, cmp func(a, b *E) int) {
	sort.Slice(slice, func(i, j int) bool {
		return cmp(&slice[i], &slice[j]) < 0
	})
}
