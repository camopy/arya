package ge

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	type A struct{ b int }
	s := "1"
	i := int32(2)

	assert.Exactly(t, &s, Ptr("1"))
	assert.Exactly(t, &i, Ptr[int32](2))
	assert.Exactly(t, &A{3}, Ptr(A{3}))
}

func TestCond(t *testing.T) {
	assert.Exactly(t, "true", Cond(true, "true", "false"))
	assert.Exactly(t, "false", Cond(false, "true", "false"))
}
