/*
This file contains some functions for error handling. Note that we are moving
towards using x.Trace, i.e., rpc tracing using net/tracer. But for now, these
functions are useful for simple checks logged on one machine.

Some common use cases are:
(1) You receive an error from external lib, and would like to log / check fatal.
    For this, use x.Check, x.Checkf. These will check for err != nil, which is
		more common in Go. If you want to check for boolean being true, use
		x.Assert, x.Assertf.
(2) You receive an error from external lib, and would like to pass on, with some
    stack trace information. In this case, use x.Wrap or x.Wrapf.
(3) You want to generate a new error with stack trace information. Use x.Errorf.
*/
package x

import (
	"log"

	"github.com/pkg/errors"
)

// Check logs fatal if err != nil.
func Check(err error) {
	if err != nil {
		log.Fatalf("%+v\n", errors.Wrap(err, ""))
	}
}

// Checkf is Check with extra info.
func Checkf(err error, format string, args ...interface{}) {
	if err != nil {
		log.Fatalf("%+v\n", errors.Wrapf(err, format, args))
	}
}

// Assert logs fatal if b is false.
func Assert(b bool) {
	if !b {
		log.Fatalf("%+v\n", errors.Errorf("Assert failed"))
	}
}

// Assertf is Assert with extra info.
func Assertf(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v\n", errors.Errorf(format, args))
	}
}

// Wrap wraps errors from external lib.
func Wrap(err error) error {
	return errors.Wrap(err, "")
}

// Wrapf is Wrap with extra info.
func Wrapf(err error, format string, args ...interface{}) error {
	return errors.Wrapf(err, format, args)
}

// Errorf creates a new error with stack trace, etc.
func Errorf(format string, args ...interface{}) error {
	return errors.Errorf(format, args)
}
