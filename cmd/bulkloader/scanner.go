package main

import (
	"bufio"
	"bytes"
	"io"
)

type scanner struct {
	err error
	r   *bufio.Reader
	buf bytes.Buffer
}

func newScanner(r io.Reader) *scanner {
	return &scanner{r: bufio.NewReader(r)}
}

func (s *scanner) Scan() bool {
	s.buf.Reset()
	isPrefix := true
	for isPrefix && s.err == nil {
		var line []byte
		line, isPrefix, s.err = s.r.ReadLine()
		if s.err == nil {
			s.buf.Write(line)
		}
	}
	return s.err == nil || s.err == io.EOF
}

func (s *scanner) Text() string {
	return string(s.buf.Bytes())
}

func (s *scanner) Err() error {
	return s.err
}
