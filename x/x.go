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
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

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

type Status struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DirectedEdge struct {
	Entity     uint64 // Subject or source node / UID.
	Attribute  string // Attribute or predicate. Labels the edge.
	Value      []byte // Edge points to a value.
	ValueType  byte   // The type of the value
	ValueId    uint64 // Object or destination node / UID.
	Source     string
	IndexToken string // If nonempty, this is an index mutation. Key is Attr and IndexToken.
	Timestamp  time.Time
}

// Mutations stores the directed edges for both the set and delete operations.
type Mutations struct {
	GroupId uint32
	Set     []DirectedEdge
	Del     []DirectedEdge
}

// Encode gob encodes the mutation which is then sent over to the instance which
// is supposed to run it.
func (m *Mutations) Encode() (data []byte, rerr error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	rerr = enc.Encode(*m)
	return b.Bytes(), rerr
}

// Decode decodes the mutation from a byte slice after receiving the byte slice over
// the network.
func (m *Mutations) Decode(data []byte) error {
	r := bytes.NewReader(data)
	dec := gob.NewDecoder(r)
	return dec.Decode(m)
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

// PageRange returns range for a list of n elements given pagination params.
func PageRange(offset int, count int, n int) (int, int) {
	if count == 0 && offset == 0 {
		return 0, n
	}
	if count < 0 {
		// Items from the back of the array, like Python arrays. Do a postive mod n.
		return (((n + count) % n) + n) % n, n
	}
	start := offset
	if start < 0 {
		start = 0
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
