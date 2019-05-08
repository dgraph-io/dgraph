// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadog.com/).
// Copyright 2018 Datadog, Inc.

//go:generate msgp -unexported -marshal=false -o=msgpack_gen.go -tests=false
//msgp:ignore payload packedSpans

package datadog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tinylib/msgp/msgp"
)

type (
	ddPayload []ddTrace // used in tests
	ddTrace   []ddSpan  // used in tests
)

// ddSpan represents the Datadog span definition.
type ddSpan struct {
	SpanID   uint64             `msg:"span_id"`
	TraceID  uint64             `msg:"trace_id"`
	ParentID uint64             `msg:"parent_id"`
	Name     string             `msg:"name"`
	Service  string             `msg:"service"`
	Resource string             `msg:"resource"`
	Type     string             `msg:"type"`
	Start    int64              `msg:"start"`
	Duration int64              `msg:"duration"`
	Meta     map[string]string  `msg:"meta,omitempty"`
	Metrics  map[string]float64 `msg:"metrics,omitempty"`
	Error    int32              `msg:"error"`
}

// maxLength indicates the maximum number of items supported in a msgpack-encoded array.
// See: https://github.com/msgpack/msgpack/blob/master/spec.md#array-format-family
const maxLength = uint(math.MaxUint32)

// errOverflow is returned when maxLength is exceeded.
var errOverflow = fmt.Errorf("maximum msgpack array length (%d) exceeded", maxLength)

// payload represents a Datadog-compatible, msgpack-encoded payload consisting of traces.
// It allows adding spans sequentially while keeping track of the size of the resulting payload.
type payload struct {
	// traces maps trace IDs to their specific set of msgpack-encoded spans.
	traces map[uint64]*packedSpans

	// headerlessSize specifies the size of the payload in bytes, excluding the header
	// which can range between 1 to 5 bytes, depending on len(traces).
	headerlessSize int
}

func newPayload() *payload {
	return &payload{traces: make(map[uint64]*packedSpans)}
}

// reset resets the payload, making it ready to use for a new buffer.
func (p *payload) reset() {
	p.traces = make(map[uint64]*packedSpans)
	p.headerlessSize = 0
}

// size returns the number of bytes that the resulting payload would occupy given
// the current state.
func (p *payload) size() int {
	return p.headerlessSize + arrayHeaderSize(uint64(len(p.traces)))
}

// add adds the given span to the payload.
func (p *payload) add(span *ddSpan) error {
	if uint(len(p.traces)) >= maxLength {
		return errOverflow
	}
	id := span.TraceID
	if _, ok := p.traces[id]; !ok {
		p.traces[id] = new(packedSpans)
	}
	oldsize := p.traces[id].size()
	if err := p.traces[id].add(span); err != nil {
		return err
	}
	newsize := p.traces[id].size()
	p.headerlessSize += newsize - oldsize
	return nil
}

// buffer creates a copy of the msgpack-encoded payload and returns it.
func (p *payload) buffer() *bytes.Buffer {
	var (
		buf    bytes.Buffer
		header [8]byte
	)
	off := arrayHeader(&header, uint64(len(p.traces)))
	buf.Write(header[off:])
	for _, ss := range p.traces {
		buf.Write(ss.bytes())
	}
	return &buf
}

// packedSpans represents a slice of spans encoded in msgpack format. It allows adding spans
// sequentially while keeping track of their count.
type packedSpans struct {
	count uint64       // number of items in slice
	buf   bytes.Buffer // msgpack encoded items (without header)
}

// add adds the given span to the trace.
func (s *packedSpans) add(span *ddSpan) error {
	if uint(s.count) >= maxLength {
		return errOverflow
	}
	if err := msgp.Encode(&s.buf, span); err != nil {
		return err
	}
	s.count++
	return nil
}

// size returns the number of bytes that would be returned by a call to bytes().
func (s *packedSpans) size() int {
	return s.buf.Len() + arrayHeaderSize(s.count)
}

// reset resets the packedSpans.
func (s *packedSpans) reset() {
	s.count = 0
	s.buf.Reset()
}

// bytes returns the msgpack encoded set of bytes that represents the entire slice.
func (s *packedSpans) bytes() []byte {
	var header [8]byte
	off := arrayHeader(&header, s.count)
	var buf bytes.Buffer
	buf.Write(header[off:])
	buf.Write(s.buf.Bytes())
	return buf.Bytes()
}

// arrayHeader writes the msgpack array header for a slice of length n into out.
// It returns the offset at which to begin reading from out. For more information,
// see the msgpack spec:
// https://github.com/msgpack/msgpack/blob/master/spec.md#array-format-family
func arrayHeader(out *[8]byte, n uint64) (off int) {
	const (
		msgpackArrayFix byte = 144  // up to 15 items
		msgpackArray16       = 0xdc // up to 2^16-1 items, followed by size in 2 bytes
		msgpackArray32       = 0xdd // up to 2^32-1 items, followed by size in 4 bytes
	)
	off = 8 - arrayHeaderSize(n)
	switch {
	case n <= 15:
		out[off] = msgpackArrayFix + byte(n)
	case n <= math.MaxUint16:
		binary.BigEndian.PutUint64(out[:], n) // writes 2 bytes
		out[off] = msgpackArray16
	case n <= math.MaxUint32:
		fallthrough
	default:
		binary.BigEndian.PutUint64(out[:], n) // writes 4 bytes
		out[off] = msgpackArray32
	}
	return off
}

// arrayHeaderSize returns the size in bytes of a header for a msgpack array of length n.
func arrayHeaderSize(n uint64) int {
	switch {
	case n == 0:
		return 0
	case n <= 15:
		return 1
	case n <= math.MaxUint16:
		return 3
	case n <= math.MaxUint32:
		fallthrough
	default:
		return 5
	}
}
