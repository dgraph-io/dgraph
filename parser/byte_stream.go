package parser

import (
	"bufio"
	"io"
)

type byteStream struct {
	r    *bufio.Reader
	err  error
	next *byteStream
	b    byte
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
		bs.next = &byteStream{
			r: bs.r,
		}
		bs.next.init()
	}
	return bs.next
}

type byteToken byte

func (bt byteToken) Value() interface{} { return byte(bt) }

func (bs *byteStream) Token() Token {
	if bs.err != nil {
		panic(bs.err)
	}
	return byteToken(bs.b)
}

func NewByteStream(r io.Reader) Stream {
	bs := byteStream{
		r: bufio.NewReader(r),
	}
	bs.init()
	return &bs
}
