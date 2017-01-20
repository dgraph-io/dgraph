/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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

	"github.com/Sirupsen/logrus"
	"golang.org/x/net/trace"
)

// Error constants representing different types of errors.
const (
	ErrorOk              = "ErrorOk"
	ErrorUnauthorized    = "ErrorUnauthorized"
	ErrorInvalidMethod   = "ErrorInvalidMethod"
	ErrorInvalidRequest  = "ErrorInvalidRequest"
	ErrorMissingRequired = "ErrorMissingRequired"
	Error                = "Error"
	ErrorNoData          = "ErrorNoData"
	ErrorUptodate        = "ErrorUptodate"
	ErrorNoPermission    = "ErrorNoPermission"
	ErrorInvalidMutation = "ErrorInvalidMutation"
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

func Log(p string) *logrus.Entry {
	l := logrus.WithFields(logrus.Fields{
		"package": p,
	})
	return l
}

func Err(entry *logrus.Entry, err error) *logrus.Entry {
	return entry.WithField("error", err)
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
