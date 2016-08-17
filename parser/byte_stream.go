package parser

import (
	"bufio"
	"fmt"
	"io"

	"github.com/joeshaw/gengen/generic"
)

type byteStream struct {
	r          *bufio.Reader
	err        error
	next       *byteStream
	b          byte
	sourceName string
	line       int
	col        int
}

func (bs *byteStream) init() {
	bs.b, bs.err = bs.r.ReadByte()
}

func (bs byteStream) Err() error {
	return bs.err
}

func (bs byteStream) Good() bool {
	return bs.err == nil
}

func (bs *byteStream) Next() Stream {
	if bs.err != nil {
		return bs
	}
	if bs.next == nil {
		next := *bs
		if bs.b == '\n' {
			next.line++
			next.col = 1
		} else {
			next.col++
		}
		next.init()
		bs.next = &next
	}
	return bs.next
}

type byteToken struct {
	b byte
	s *byteStream
}

func (bs *byteStream) Position() interface{} {
	return fmt.Sprintf("%s:%d:%d", bs.sourceName, bs.line, bs.col)
}

func (bs *byteStream) Token() generic.T {
	return bs.b
}

func NewByteStream(r io.Reader) Stream {
	bs := byteStream{
		r:    bufio.NewReader(r),
		line: 1,
		col:  1,
	}
	bs.init()
	return &bs
}
