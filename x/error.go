/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
//     stack trace information. In this case, use x.Wrap or errors.Wrapf.
// (3) You want to generate a new error with stack trace info. Use errors.Errorf.

import (
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"
)

// Check logs fatal if err != nil.
func Check(err error) {
	if err != nil {
		log.Fatalf("%+v", errors.Wrap(err, ""))
	}
}

// Checkf is Check with extra info.
func Checkf(err error, format string, args ...interface{}) {
	if err != nil {
		log.Fatalf("%+v", errors.Wrapf(err, format, args...))
	}
}

// CheckfNoTrace is Checkf without a stack trace.
func CheckfNoTrace(err error) {
	if err != nil {
		log.Fatalf(err.Error())
	}
}

// CheckfNoLog exits on error without any message (to avoid duplicate error messages).
func CheckfNoLog(err error) {
	if err != nil {
		os.Exit(1)
	}
}

// Check2 acts as convenience wrapper around Check, using the 2nd argument as error.
func Check2(_ interface{}, err error) {
	Check(err)
}

// Ignore function is used to ignore errors deliberately, while keeping the
// linter happy.
func Ignore(_ error) {
	// Do nothing.
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

// AssertTruefNoTrace is AssertTruef without a stack trace.
func AssertTruefNoTrace(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v", fmt.Errorf(format, args...))
	}
}

// Fatalf logs fatal.
func Fatalf(format string, args ...interface{}) {
	log.Fatalf("%+v", errors.Errorf(format, args...))
}
