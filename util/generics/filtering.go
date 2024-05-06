package ge

func Filter[V any](slice []V, predicate func(it V) bool) []V {
	if len(slice) == 0 {
		return nil
	}
	res := make([]V, 0, len(slice))
	for i := range slice {
		if predicate(slice[i]) {
			res = append(res, slice[i])
		}
	}
	return res
}

func FilterPtr[V any](slice []V, predicate func(it *V) bool) []V {
	if len(slice) == 0 {
		return nil
	}
	res := make([]V, 0, len(slice))
	for i := range slice {
		if predicate(&slice[i]) {
			res = append(res, slice[i])
		}
	}
	return res
}
