/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

// This file contains some functions for error handling. Note that we are moving
// towards using x.Trace, i.e., rpc tracing using net/tracer. But for now, these
// functions are useful for simple checks logged on one machine.
// Some common use cases are:
// (1) You receive an error from external lib, and would like to check/log fatal.
//     For this, use x.Check, x.Checkf. These will check for err != nil, which is
//     more common in Go. If you want to check for boolean being true, use
//		   x.Assert, x.Assertf.
// (2) You receive an error from external lib, and would like to pass on with some
//     stack trace information. In this case, use x.Wrap or x.Wrapf.
// (3) You want to generate a new error with stack trace info. Use x.Errorf.

import (
	"fmt"
	"log"

	"github.com/pkg/errors"
)

// Check logs fatal if err != nil.
func Check(err error) {
	if err != nil {
		log.Fatalf("%+v", Wrap(err))
	}
}

// Checkf is Check with extra info.
func Checkf(err error, format string, args ...interface{}) {
	if err != nil {
		log.Fatalf("%+v", Wrapf(err, format, args...))
	}
}

func CheckfNoTrace(err error) {
	if err != nil {
		log.Fatalf(err.Error())
	}
}

// Check2 acts as convenience wrapper around Check, using the 2nd argument as error.
func Check2(_ interface{}, err error) {
	Check(err)
}

// AssertTrue asserts that b is true. Otherwise, it would log fatal.
func AssertTrue(b bool) {
	if !b {
		log.Fatalf("%+v", errors.Errorf("Assert failed"))
	}
}

// AssertTruef is AssertTrue with extra info.
func AssertTruef(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v", errors.Errorf(format, args...))
	}
}

func AssertTruefNoTrace(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v", fmt.Errorf(format, args...))
	}
}

// Wrap wraps errors from external lib.
func Wrap(err error) error {
	return errors.Wrap(err, "")
}

// Wrapf is Wrap with extra info.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if !Config.DebugMode {
		return fmt.Errorf(format+" error: %+v", append(args, err)...)
	}
	return errors.Wrapf(err, format, args...)
}

// Errorf creates a new error with stack trace, etc.
func Errorf(format string, args ...interface{}) error {
	if !Config.DebugMode {
		return fmt.Errorf(format, args...)
	}
	return errors.Errorf(format, args...)
}

// Fatalf logs fatal.
func Fatalf(format string, args ...interface{}) {
	log.Fatalf("%+v", errors.Errorf(format, args...))
}
