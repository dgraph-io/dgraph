/*
 * Copyright 2016 DGraph Labs, Inc.
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
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/task"
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
	ErrorInvalidQuery    = "ErrorInvalidQuery"
)

const dgraphVersion = "0.4.3"

var version = flag.Bool("version", false, "Prints the version of Dgraph")

type Status struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DirectedEdge struct {
	Entity    uint64
	Attribute string
	Value     []byte
	ValueId   uint64
	Source    string
	Timestamp time.Time
}

// PrintVersionOnly prints version and other helpful information
// if version flag is set to true.
func PrintVersionOnly() bool {
	if *version {
		fmt.Printf("Dgraph version %s\n", dgraphVersion)
		fmt.Println("\nCopyright 2016 Dgraph Labs, Inc.")
		fmt.Println("Licensed under the Apache License, Version 2.0.")
		fmt.Println("\nFor Dgraph official documentation, visit https://wiki.dgraph.io.")
		fmt.Println("For discussions about Dgraph, visit https://discuss.dgraph.io.")
		fmt.Println("To say hi to the community, visit https://dgraph.slack.com.\n")
		return true
	}

	return false
}

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

func SetStatus(w http.ResponseWriter, code, msg string) {
	r := &Status{Code: code, Message: msg}
	if js, err := json.Marshal(r); err == nil {
		fmt.Fprint(w, string(js))
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

func UidlistOffset(b *flatbuffers.Builder,
	sorted []uint64) flatbuffers.UOffsetT {

	task.UidListStartUidsVector(b, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		b.PrependUint64(sorted[i])
	}
	ulist := b.EndVector(len(sorted))
	task.UidListStart(b)
	task.UidListAddUids(b, ulist)
	return task.UidListEnd(b)
}

var Nilbyte []byte

func Trace(ctx context.Context, format string, args ...interface{}) {
	tr, ok := trace.FromContext(ctx)
	if !ok {
		return
	}
	tr.LazyPrintf(format, args...)
}
