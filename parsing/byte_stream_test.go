package parsing

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByteStream(t *testing.T) {
	bs := NewByteStream(bytes.NewBufferString("<a> \n\x00"))
	// First token.
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token())
	assert.Equal(t, ":1:1", bs.Position())
	// Second token.
	bs1 := bs.Next()
	assert.NoError(t, bs1.Err())
	assert.Equal(t, byte('a'), bs1.Token())
	assert.Equal(t, ":1:2", bs1.Position())
	// Check that the first token is unchanged.
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token())
	// Check the EOF token.
	bsEOF := bs1.Next().Next().Next().Next().Next()
	assert.Equal(t, io.EOF, bsEOF.Err())
	assert.Equal(t, ":2:2", bsEOF.Position())
	// Check that .Next on the error token returns the same value.
	bsEOFNext := bsEOF.Next()
	assert.Equal(t, io.EOF, bsEOFNext.Err())
	assert.Equal(t, ":2:2", bsEOFNext.Position())
}

func TestByteStreamEmpty(t *testing.T) {
	bs := NewByteStream(bytes.NewBufferString(""))
	assert.Equal(t, io.EOF, bs.Err())
	assert.Equal(t, ":1:1", bs.Position())
	bs1 := bs.Next()
	assert.Equal(t, io.EOF, bs1.Err())
	assert.Equal(t, ":1:1", bs.Position())
}
