package ge

func GroupBy[T any, K comparable](slice []T, groupKeySelector func(it T) K) map[K][]T {
	if len(slice) == 0 {
		return nil
	}

	groups := map[K][]T{}
	for i := range slice {
		key := groupKeySelector(slice[i])
		groups[key] = append(groups[key], slice[i])
	}
	return groups
}

func GroupByPtr[T any, K comparable](slice []T, groupKeySelector func(it *T) K) map[K][]T {
	if len(slice) == 0 {
		return nil
	}

	groups := map[K][]T{}
	for i := range slice {
		key := groupKeySelector(&slice[i])
		groups[key] = append(groups[key], slice[i])
	}
	return groups
}

func OrderedGroupBy[T any, K comparable](slice []T, groupKeySelector func(it T) K) [][]T {
	if len(slice) == 0 {
		return nil
	}

	keys := make([]K, 0, len(slice))
	groups := map[K][]T{}
	for i := range slice {
		key := groupKeySelector(slice[i])
		if group, ok := groups[key]; ok {
			groups[key] = append(group, slice[i])
		} else {
			groups[key] = []T{slice[i]}
			keys = append(keys, key)
		}
	}

	res := make([][]T, 0, len(groups))
	for i := range keys {
		res = append(res, groups[keys[i]])
	}
	return res
}

func OrderedGroupByPtr[T any, K comparable](slice []T, groupKeySelector func(it *T) K) [][]T {
	if len(slice) == 0 {
		return nil
	}

	keys := make([]K, 0, len(slice))
	groups := map[K][]T{}
	for i := range slice {
		key := groupKeySelector(&slice[i])
		if group, ok := groups[key]; ok {
			groups[key] = append(group, slice[i])
		} else {
			groups[key] = []T{slice[i]}
			keys = append(keys, key)
		}
	}

	res := make([][]T, 0, len(groups))
	for i := range keys {
		res = append(res, groups[keys[i]])
	}
	return res
}

func SortedGroupBy[T any, K comparable](sortedSlice []T, groupKeySelector func(it T) K) [][]T {
	if len(sortedSlice) == 0 {
		return nil
	}
	res := make([][]T, 0, len(sortedSlice))
	var lastGroup *[]T
	var lastKey K
	for i := range sortedSlice {
		key := groupKeySelector(sortedSlice[i])
		if lastGroup == nil || lastKey != key {
			res = append(res, []T{sortedSlice[i]})
			lastGroup = &res[len(res)-1]
			lastKey = key
		} else {
			*lastGroup = append(*lastGroup, sortedSlice[i])
		}
	}
	return res
}

func SortedGroupByPtr[T any, K comparable](sortedSlice []T, groupKeySelector func(it *T) K) [][]T {
	if len(sortedSlice) == 0 {
		return nil
	}
	res := make([][]T, 0, len(sortedSlice))
	var lastGroup *[]T
	var lastKey K
	for i := range sortedSlice {
		key := groupKeySelector(&sortedSlice[i])
		if lastGroup == nil || lastKey != key {
			res = append(res, []T{sortedSlice[i]})
			lastGroup = &res[len(res)-1]
			lastKey = key
		} else {
			*lastGroup = append(*lastGroup, sortedSlice[i])
		}
	}
	return res
}
