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
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/task"
	"github.com/google/flatbuffers/go"
)

const (
	E_OK               = "E_OK"
	E_UNAUTHORIZED     = "E_UNAUTHORIZED"
	E_INVALID_METHOD   = "E_INVALID_METHOD"
	E_INVALID_REQUEST  = "E_INVALID_REQUEST"
	E_MISSING_REQUIRED = "E_MISSING_REQUIRED"
	E_ERROR            = "E_ERROR"
	E_NODATA           = "E_NODATA"
	E_UPTODATE         = "E_UPTODATE"
	E_NOPERMISSION     = "E_NOPERMISSION"

	DUMMY_UUID = "00000000-0000-0000-0000-000000000000"
)

type Status struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DirectedEdge struct {
	Entity    uint64
	Attribute string
	Value     interface{}
	ValueId   uint64
	Source    string
	Timestamp time.Time
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
		SetStatus(w, E_ERROR, "Internal server error")
	}
}

func ParseRequest(w http.ResponseWriter, r *http.Request, data interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&data); err != nil {
		SetStatus(w, E_ERROR, fmt.Sprintf("While parsing request: %v", err))
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

func init() {
	Nilbyte = make([]byte, 1)
	Nilbyte[0] = 0x00
}
