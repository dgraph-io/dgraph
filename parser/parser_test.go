package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
}

func TestNoMatchEquality(t *testing.T) {
	assert.False(t, &struct{}{} == NoMatch)
	assert.True(t, NoMatch == NoMatch)
}
