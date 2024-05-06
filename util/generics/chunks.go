package ge

func Chunks[T any](slice []T, chunkSize int) [][]T {
	count := len(slice)
	if count == 0 {
		return nil
	}
	res := make([][]T, 0, (count+chunkSize-1)/chunkSize)
	for from := 0; from < count; from += chunkSize {
		to := from + chunkSize
		if to > count {
			to = count
		}
		res = append(res, slice[from:to])
	}
	return res
}
