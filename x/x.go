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

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/trace"
)

// Error constants representing different types of errors.
const (
	Success              = "Success"
	ErrorUnauthorized    = "ErrorUnauthorized"
	ErrorInvalidMethod   = "ErrorInvalidMethod"
	ErrorInvalidRequest  = "ErrorInvalidRequest"
	ErrorMissingRequired = "ErrorMissingRequired"
	Error                = "Error"
	ErrorNoData          = "ErrorNoData"
	ErrorUptodate        = "ErrorUptodate"
	ErrorNoPermission    = "ErrorNoPermission"
	ErrorInvalidMutation = "ErrorInvalidMutation"
	DeleteAll            = "_DELETE_POSTING_"
)

var (
	debugMode = flag.Bool("debugmode", false,
		"enable debug mode for more debug information")
)

// WhiteSpace Replacer removes spaces and tabs from a string.
var WhiteSpace = strings.NewReplacer(" ", "", "\t", "")

type Status struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SetError sets the error logged in this package.
func SetError(prev *error, n error) {
	if prev == nil {
		prev = &n
	}
}

// SetStatus sets the error code, message and the newly assigned uids
// in the http response.
func SetStatus(w http.ResponseWriter, code, msg string) {
	r := &Status{Code: code, Message: msg}
	if js, err := json.Marshal(r); err == nil {
		w.Write(js)
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", r))
	}
}

func Reply(w http.ResponseWriter, rep interface{}) {
	if js, err := json.Marshal(rep); err == nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(js))
	} else {
		SetStatus(w, Error, "Internal server error")
	}
}

func ParseRequest(w http.ResponseWriter, r *http.Request, data interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&data); err != nil {
		SetStatus(w, Error, fmt.Sprintf("While parsing request: %v", err))
		return false
	}
	return true
}

var Nilbyte []byte

func Trace(ctx context.Context, format string, args ...interface{}) {
	if *debugMode {
		fmt.Printf(format+"\n", args...)
		return
	}

	tr, ok := trace.FromContext(ctx)
	if !ok {
		return
	}
	tr.LazyPrintf(format, args...)
}

// Reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func ReadLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	buf.Reset()
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an internal buffer in bufio and is only
		// valid until the next call to ReadLine. It needs to be copied
		// over to our own buffer.
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			buf.Write(line)
		}
	}
	return err
}

// Go doesn't have a round function for Duration and the duration value can be a
// bit ugly to look at sometimes. This is an attempt to round the value.
func Round(d time.Duration) time.Duration {
	var denominator time.Duration
	if d > time.Minute {
		denominator = 0.1e12 // So that it has one digit after the decimal.
	} else if d > time.Second {
		denominator = 0.1e9 // 0.1 * time.Second
	} else if d > time.Millisecond {
		denominator = time.Millisecond
	} else {
		denominator = time.Microsecond
	}

	if remainder := d % denominator; 2*remainder < denominator {
		// This means we have to round it down.
		d = d - remainder
	} else {
		d = d + denominator - remainder
	}
	return d
}

// PageRange returns start and end indices given pagination params. Note that n
// is the size of the input list.
func PageRange(count, offset, n int) (int, int) {
	if n == 0 {
		return 0, 0
	}
	if count == 0 && offset == 0 {
		return 0, n
	}
	if count < 0 {
		// Items from the back of the array, like Python arrays. Do a positive mod n.
		if count*-1 > n {
			count = -n
		}
		return (((n + count) % n) + n) % n, n
	}
	start := offset
	if start < 0 {
		start = 0
	}
	if start > n {
		return n, n
	}
	if count == 0 { // No count specified. Just take the offset parameter.
		return start, n
	}
	end := start + count
	if end > n {
		end = n
	}
	return start, end
}
