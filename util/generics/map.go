package ge

func MKeys[K comparable, V any](m map[K]V) []K {
	if len(m) == 0 {
		return nil
	}
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func MValues[K comparable, V any](m map[K]V) []V {
	if len(m) == 0 {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

func KeyBy[T any, K comparable](slice []T, keySelector func(it T) K) map[K]T {
	if len(slice) == 0 {
		return nil
	}
	res := map[K]T{}
	for i := range slice {
		res[keySelector(slice[i])] = slice[i]
	}
	return res
}

func KeyByPtr[T any, K comparable](slice []T, keySelector func(it *T) K) map[K]T {
	if len(slice) == 0 {
		return nil
	}
	res := map[K]T{}
	for i := range slice {
		res[keySelector(&slice[i])] = slice[i]
	}
	return res
}

func SliceToMap[T any, K comparable, V any](slice []T, selector func(it T) (K, V)) map[K]V {
	if len(slice) == 0 {
		return nil
	}
	res := map[K]V{}
	for i := range slice {
		key, value := selector(slice[i])
		res[key] = value
	}
	return res
}

func SliceToMapPtr[T any, K comparable, V any](slice []T, selector func(it *T) (K, V)) map[K]V {
	if len(slice) == 0 {
		return nil
	}
	res := map[K]V{}
	for i := range slice {
		key, value := selector(&slice[i])
		res[key] = value
	}
	return res
}
