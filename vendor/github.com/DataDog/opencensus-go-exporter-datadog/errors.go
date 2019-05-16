// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadog.com/).
// Copyright 2018 Datadog, Inc.

package datadog

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	// defaultErrorLimit specifies the maximum number of occurrences that will
	// be recorded for an error of a certain type.
	defaultErrorLimit = 50

	// defaultErrorFreq specifies the default frequency at which errors will
	// be reported.
	defaultErrorFreq = 5 * time.Second
)

// errorType specifies the error type.
type errorType int

const (
	// errorTypeEncoding specifies that an encoding error has occurred.
	errorTypeEncoding errorType = iota

	// errorTypeOverflow specifies that the in channel capacity has been reached.
	errorTypeOverflow

	// errorTypeTransport specifies that an error occurred while trying
	// to upload spans to the agent.
	errorTypeTransport

	// errorTypeUnknown specifies that an unknown error type was reported.
	errorTypeUnknown
)

// errorTypeStrings maps error types to their human-readable description.
var errorTypeStrings = map[errorType]string{
	errorTypeEncoding:  "encoding error",
	errorTypeOverflow:  "span buffer overflow",
	errorTypeTransport: "transport error",
	errorTypeUnknown:   "error",
}

// String implements fmt.Stringer.
func (et errorType) String() string { return errorTypeStrings[et] }

// errorAmortizer amortizes high frequency errors and condenses them into
// periodical reports to avoid flooding.
type errorAmortizer struct {
	interval time.Duration // frequency of report
	callback func(error)   // error handler; defaults to log.Println

	mu      sync.RWMutex // guards below fields
	pausing bool
	errs    map[errorType]*aggregateError
}

// newErrorAmortizer creates a new errorAmortizer which calls the provided function
// at the given interval, passing it a detailed error report if one has occurred.
func newErrorAmortizer(interval time.Duration, cb func(error)) *errorAmortizer {
	if cb == nil {
		cb = func(err error) {
			log.Println(err)
		}
	}
	return &errorAmortizer{
		interval: interval,
		callback: cb,
		errs:     make(map[errorType]*aggregateError),
	}
}

// flush flushes any aggregated errors and resets the amortizer.
func (e *errorAmortizer) flush() {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(e.errs)
	if n == 0 {
		return
	}
	var str strings.Builder
	str.WriteString("Datadog Exporter error: ")
	for _, err := range e.errs {
		if n > 1 {
			str.WriteString("\n\t")
		}
		str.WriteString(err.Error())
	}
	e.callback(errors.New(str.String()))
	e.errs = make(map[errorType]*aggregateError)
	e.pausing = false
}

// limitReached returns true if the defaultErrorLimit has been reached
// for the given error type.
func (e *errorAmortizer) limitReached(typ errorType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errs[typ] != nil && e.errs[typ].num > defaultErrorLimit-1
}

// log logs an error of the given type, having the given message. err
// is optional and can be nil.
func (e *errorAmortizer) log(typ errorType, err error) {
	if e.limitReached(typ) {
		// avoid too much lock contention
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.errs[typ]; !ok {
		e.errs[typ] = newError(typ, err)
	} else {
		e.errs[typ].num++
	}
	if !e.pausing {
		e.pausing = true
		time.AfterFunc(e.interval, e.flush)
	}
}

var _ error = (*aggregateError)(nil)

// aggregateError is an error consisting of a type and an optional context
// error. It is used to aggregate errors inside the errorAmortizer.
type aggregateError struct {
	typ errorType // error type
	err error     // error message (optional)
	num int       // number of occurrences
}

// newError creates a new aggregateError.
func newError(t errorType, err error) *aggregateError {
	return &aggregateError{t, err, 1}
}

// Error implements the error interface. If the error occurred more than
// once, it appends the number of occurrences to the error message.
func (e *aggregateError) Error() string {
	var str strings.Builder
	if e.err == nil {
		str.WriteString(e.typ.String())
	} else {
		// no need to include the type into the message, it will be evident
		// from the message itself.
		str.WriteString(e.err.Error())
	}
	if e.num >= defaultErrorLimit {
		str.WriteString(fmt.Sprintf(" (x%d+)", defaultErrorLimit))
	} else if e.num > 1 {
		str.WriteString(fmt.Sprintf(" (x%d)", e.num))
	}
	return str.String()
}
