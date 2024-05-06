package errutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFatalError(t *testing.T) {
	assert.False(t, IsFatalError(nil))
	assert.False(t, IsFatalError(context.Canceled))
	assert.False(t, IsFatalError(FatalError(context.Canceled)))
}
