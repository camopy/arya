package ge

import (
	"cmp"
	"testing"
)

func BenchmarkCompareMulti(b *testing.B) {
	type logRef struct {
		BlockNumber uint32
		LogIndex    uint16
		TopicIndex  uint8
	}

	compareManual := func(r, r2 logRef) int {
		if r.BlockNumber == r2.BlockNumber {
			if r.LogIndex == r2.LogIndex {
				if r.TopicIndex == r2.TopicIndex {
					return 0
				}
				if r.TopicIndex < r2.TopicIndex {
					return -1
				}
				return 1
			}
			if r.LogIndex < r2.LogIndex {
				return -1
			}
			return 1
		}
		if r.BlockNumber < r2.BlockNumber {
			return -1
		}
		return 1
	}

	compareUtil := func(r, r2 logRef) int {
		return CompareMulti(
			cmp.Compare(r.BlockNumber, r2.BlockNumber),
			cmp.Compare(r.LogIndex, r2.LogIndex),
			cmp.Compare(r.TopicIndex, r2.TopicIndex),
		)
	}

	b.Run("manual", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r := logRef{
				BlockNumber: uint32(i),
				LogIndex:    uint16(i),
				TopicIndex:  uint8(i),
			}
			r2 := logRef{
				BlockNumber: uint32(i * i),
				LogIndex:    uint16(i * i),
				TopicIndex:  uint8(i * i),
			}
			_ = compareManual(r, r2)
		}
	})

	b.Run("util", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r := logRef{
				BlockNumber: uint32(i),
				LogIndex:    uint16(i),
				TopicIndex:  uint8(i),
			}
			r2 := logRef{
				BlockNumber: uint32(i * i),
				LogIndex:    uint16(i * i),
				TopicIndex:  uint8(i * i),
			}
			_ = compareUtil(r, r2)
		}
	})
}
