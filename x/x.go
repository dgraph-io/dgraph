/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/trace"
)

// Error constants representing different types of errors.
const (
	Success                 = "Success"
	ErrorUnauthorized       = "ErrorUnauthorized"
	ErrorInvalidMethod      = "ErrorInvalidMethod"
	ErrorInvalidRequest     = "ErrorInvalidRequest"
	ErrorMissingRequired    = "ErrorMissingRequired"
	Error                   = "Error"
	ErrorNoData             = "ErrorNoData"
	ErrorUptodate           = "ErrorUptodate"
	ErrorNoPermission       = "ErrorNoPermission"
	ErrorInvalidMutation    = "ErrorInvalidMutation"
	ErrorServiceUnavailable = "ErrorServiceUnavailable"
	ValidHostnameRegex      = "^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$"
	// When changing this value also remember to change in in client/client.go:DeleteEdges.
	Star        = "_STAR_ALL"
	GrpcMaxSize = 256 << 20
	// The attr used to store list of predicates for a node.
	PredicateListAttr = "_predicate_"

	PortInternal = 7080
	PortHTTP     = 8080
	PortGrpc     = 9080
)

var (
	// Useful for running multiple servers on the same machine.
	regExpHostName = regexp.MustCompile(ValidHostnameRegex)
)

// WhiteSpace Replacer removes spaces and tabs from a string.
var WhiteSpace = strings.NewReplacer(" ", "", "\t", "")

type errRes struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type queryRes struct {
	Errors []errRes `json:"errors"`
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
	var qr queryRes
	qr.Errors = append(qr.Errors, errRes{Code: code, Message: msg})
	if js, err := json.Marshal(qr); err == nil {
		w.Write(js)
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", qr))
	}
}

func AddCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, X-Auth-Token, "+
			"Cache-Control, X-Requested-With, X-Dgraph-CommitNow, X-Dgraph-LinRead, X-Dgraph-Vars")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

type QueryResWithData struct {
	Errors []errRes `json:"errors"`
	Data   *string  `json:"data"`
}

// In case an error was encountered after the query execution started, we have to return data
// key with null value according to GraphQL spec.
func SetStatusWithData(w http.ResponseWriter, code, msg string) {
	var qr QueryResWithData
	qr.Errors = append(qr.Errors, errRes{Code: code, Message: msg})
	// This would ensure that data key is present with value null.
	if js, err := json.Marshal(qr); err == nil {
		w.Write(js)
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", qr))
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
		// The returned line is an intern.buffer in bufio and is only
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

// ValidateAddress checks whether given address can be used with grpc dial function
func ValidateAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p >= 65536 {
		return false
	}
	if err := net.ParseIP(host); err == nil {
		return true
	}
	// try to parse as hostname as per hostname RFC
	if len(strings.Replace(host, ".", "", -1)) > 255 {
		return false
	}
	return regExpHostName.MatchString(host)
}

// sorts the slice of strings and removes duplicates. changes the input slice.
// this function should be called like: someSlice = x.RemoveDuplicates(someSlice)
func RemoveDuplicates(s []string) (out []string) {
	sort.Strings(s)
	out = s[:0]
	for i := range s {
		if i > 0 && s[i] == s[i-1] {
			continue
		}
		out = append(out, s[i])
	}
	return
}

func NewTrace(title string, ctx context.Context) (trace.Trace, context.Context) {
	tr := trace.New("Dgraph", title)
	tr.SetMaxEvents(1000)
	ctx = trace.NewContext(ctx, tr)
	return tr, ctx
}

type BytesBuffer struct {
	data [][]byte
	off  int
	sz   int
}

func (b *BytesBuffer) grow(n int) {
	if n < 128 {
		n = 128
	}
	if len(b.data) == 0 {
		b.data = append(b.data, make([]byte, n, n))
	}

	last := len(b.data) - 1
	// Return if we have sufficient space
	if len(b.data[last])-b.off >= n {
		return
	}
	sz := len(b.data[last]) * 2
	if sz > 512<<10 {
		sz = 512 << 10 // 512 KB
	}
	if sz < n {
		sz = n
	}
	b.data[last] = b.data[last][:b.off]
	b.sz += len(b.data[last])
	b.data = append(b.data, make([]byte, sz, sz))
	b.off = 0
}

// returns a slice of lenght n to be used to writing
func (b *BytesBuffer) Slice(n int) []byte {
	b.grow(n)
	last := len(b.data) - 1
	b.off += n
	b.sz += n
	return b.data[last][b.off-n : b.off]
}

func (b *BytesBuffer) Length() int {
	return b.sz
}

// Caller should ensure that o is of appropriate length
func (b *BytesBuffer) CopyTo(o []byte) int {
	offset := 0
	for i, d := range b.data {
		if i == len(b.data)-1 {
			copy(o[offset:], d[:b.off])
			offset += b.off
		} else {
			copy(o[offset:], d)
			offset += len(d)
		}
	}
	return offset
}

// Always give back <= touched bytes
func (b *BytesBuffer) TruncateBy(n int) {
	b.off -= n
	b.sz -= n
	AssertTrue(b.off >= 0 && b.sz >= 0)
}
