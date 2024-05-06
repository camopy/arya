package ge

func Map[V any, R any](slice []V, mapper func(it V) R) []R {
	if len(slice) == 0 {
		return nil
	}
	res := make([]R, len(slice))
	for i := range slice {
		res[i] = mapper(slice[i])
	}
	return res
}

func MapPtr[V any, R any](slice []V, mapper func(it *V) R) []R {
	if len(slice) == 0 {
		return nil
	}
	res := make([]R, len(slice))
	for i := range slice {
		res[i] = mapper(&slice[i])
	}
	return res
}

func FlatMap[V any, R any](slice []V, mapper func(it V, out *[]R)) []R {
	if len(slice) == 0 {
		return nil
	}
	res := make([]R, 0, len(slice))
	for i := range slice {
		mapper(slice[i], &res)
	}
	return res
}

func FlatMapPtr[V any, R any](slice []V, mapper func(it *V, out *[]R)) []R {
	if len(slice) == 0 {
		return nil
	}
	res := make([]R, 0, len(slice))
	for i := range slice {
		mapper(&slice[i], &res)
	}
	return res
}

func MapEntries[K comparable, V, R any](m map[K]V, mapper func(key K, value V) R) []R {
	if len(m) == 0 {
		return nil
	}
	res := make([]R, 0, len(m))
	for k, v := range m {
		res = append(res, mapper(k, v))
	}
	return res
}

func MapEntriesPtr[K comparable, V, R any](m map[K]V, mapper func(key K, value *V) R) []R {
	if len(m) == 0 {
		return nil
	}
	res := make([]R, 0, len(m))
	for k := range m {
		v := m[k]
		res = append(res, mapper(k, &v))
	}
	return res
}
