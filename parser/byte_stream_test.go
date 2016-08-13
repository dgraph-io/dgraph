package parser

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByteStream(t *testing.T) {
	bs := NewByteStream(bytes.NewBufferString("<a> \n\x00"))
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token())
	bs1 := bs.Next()
	assert.NoError(t, bs1.Err())
	assert.Equal(t, byte('a'), bs1.Token())
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token())
	bsEOF := bs1.Next().Next().Next().Next().Next()
	assert.Equal(t, io.EOF, bsEOF.Err())
	assert.Equal(t, io.EOF, bsEOF.Next().Err())
	// assert.Panics(t, func() { bsEOF.Token() })
}
