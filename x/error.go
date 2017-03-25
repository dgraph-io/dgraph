/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

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

// Check2 acts as convenience wrapper around Check, using the 2nd argument as error.
func Check2(_ interface{}, err error) {
	Check(err)
}

// Check2f acts as convenience wrapper around Checkf, using the 2nd argument as error.
func Check2f(_ interface{}, err error, format string, args ...interface{}) {
	Checkf(err, format, args)
}

// AssertTrue asserts that b is true. Otherwise, it would log fatal.
func AssertTrue(b bool) {
	if !b {
		log.Fatalf("%+v", Errorf("Assert failed"))
	}
}

// AssertTruef is AssertTrue with extra info.
func AssertTruef(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v", Errorf(format, args...))
	}
}

// Wrap wraps errors from external lib.
func Wrap(err error) error {
	if !*debugMode {
		return err
	}
	return errors.Wrap(err, "")
}

// Wrapf is Wrap with extra info.
func Wrapf(err error, format string, args ...interface{}) error {
	if !*debugMode {
		if err == nil {
			return nil
		}
		return fmt.Errorf(format+" error: %+v", append(args, err)...)
	}
	return errors.Wrapf(err, format, args...)
}

// Errorf creates a new error with stack trace, etc.
func Errorf(format string, args ...interface{}) error {
	if !*debugMode {
		return fmt.Errorf(format, args...)
	}
	return errors.Errorf(format, args...)
}

// Fatalf logs fatal.
func Fatalf(format string, args ...interface{}) {
	log.Fatalf("%+v", Errorf(format, args...))
}

const (
	dgraphPrefix  = "github.com/dgraph-io/dgraph/"
	runtimePrefix = "src/runtime/"
	testingPrefix = "src/testing/"
)

func writeLineRef(buf *bytes.Buffer, s string) {
	for _, p := range []string{dgraphPrefix, runtimePrefix, testingPrefix} {
		idx := strings.Index(s, p)
		if idx >= 0 {
			buf.WriteString(s[idx+len(p):])
			return
		}
	}
	buf.WriteString(strings.TrimSpace(s))
}

func shortenedErrorString(err error) string {
	buf := bytes.NewBuffer(make([]byte, 0, 40))

	// Here is a sample input:
	//	 some error
	//	 github.com/dgraph-io/dgraph/x.Errorf
	//		/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/error.go:89
	//	 main.f
	//		/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/tmp/main.go:10
	//	 main.main
	//		/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/tmp/main.go:15
	//	 runtime.main
	//		/usr/lib/go-1.7/src/runtime/proc.go:183
	//	 runtime.goexit
	//		/usr/lib/go-1.7/src/runtime/asm_amd64.s:2086

	// Here is a sample output:
	// some error; x.Errorf (x/error.go:90) main.f (x/tmp/main.go:10) main.main (x/tmp/main.go:15) runtime.main (proc.go:183) runtime.goexit (asm_amd64.s:2086)

	// First, split into lines.
	lines := strings.Split(fmt.Sprintf("%+v\n", err), "\n")

	// The tab prefix tells us whether this line is a line reference.
	// We have two states. If lineRef is true, the last line we have seen is a line
	// reference. The initial value is true.
	lineRef := true

	// Second, process each line, depending on whether it is a line ref or not.
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") {
			// This is a line reference. We want to shorten the filename and put it
			// inside parentheses.
			buf.WriteString(" (")
			writeLineRef(buf, line)
			buf.WriteString(") ")
			lineRef = true
		} else {
			// This line is not a line reference.
			if !lineRef {
				// If the previous line is not a line reference either, we want to
				// separate them by semicolons.
				buf.WriteString("; ")
			}
			// Even though it is not a line reference, it might still contain "dgraph".
			// Let's trim that away if possible.
			buf.WriteString(strings.Replace(line, dgraphPrefix, "", -1))
			lineRef = false
		}
	}
	return buf.String()
}

// TraceError is like Trace but it logs just an error, which may have stacktrace.
func TraceError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if !*debugMode {
		Trace(ctx, err.Error())
		return
	}
	Trace(ctx, shortenedErrorString(err))
}
