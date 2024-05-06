package ge

func Unique[V comparable](slice []V) []V {
	if len(slice) <= 1 {
		return slice
	}
	m := map[V]struct{}{}
	res := make([]V, 0, len(slice))
	for _, v := range slice {
		if _, ok := m[v]; !ok {
			res = append(res, v)
			m[v] = struct{}{}
		}
	}
	return res
}

func UniqueBy[V, K comparable](slice []V, selector func(v *V) K) []V {
	if len(slice) <= 1 {
		return slice
	}
	m := map[K]struct{}{}
	res := make([]V, 0, len(slice))
	for i := range slice {
		k := selector(&slice[i])
		if _, ok := m[k]; !ok {
			res = append(res, slice[i])
			m[k] = struct{}{}
		}
	}
	return res
}
