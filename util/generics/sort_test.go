package ge

import (
	"cmp"
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestUniqueSorted(t *testing.T) {
	assert.Exactly(t, []int{1, 2, 3, 4, 5}, UniqueSorted([]int{1, 2, 3, 4, 3, 2, 2, 3, 5}))
}

func TestSortDesc(t *testing.T) {
	slice := []int{1, 2, 5, 3, 4}
	SortDesc(slice)
	assert.Exactly(t, []int{5, 4, 3, 2, 1}, slice)
}

func TestSortBy(t *testing.T) {
	slice := []string{"aa", "a", "aaa", "a"}
	SortBy(slice, func(v string) int { return len(v) })
	assert.Exactly(t, []string{"a", "a", "aa", "aaa"}, slice)
}

func TestSortByDesc(t *testing.T) {
	slice := []string{"aa", "a", "aaa", "a"}
	SortByDesc(slice, func(v string) int { return len(v) })
	assert.Exactly(t, []string{"aaa", "aa", "a", "a"}, slice)
}

func TestSortByComparator(t *testing.T) {
	slice := []int{4, 2, 5, 3, 1}
	SortByComparator(slice, cmp.Compare[int])
	assert.Exactly(t, []int{1, 2, 3, 4, 5}, slice)
}

func TestSortByComparatorPtr(t *testing.T) {
	slice := []int{4, 2, 5, 3, 1}
	SortByComparatorPtr(slice, func(a, b *int) int {
		return cmp.Compare(*a, *b)
	})
	assert.Exactly(t, []int{1, 2, 3, 4, 5}, slice)
}
