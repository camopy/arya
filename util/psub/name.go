package psub

import (
	"fmt"
	"sync/atomic"
)

var nextNameId uint64

func generateName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddUint64(&nextNameId, 1))
}
