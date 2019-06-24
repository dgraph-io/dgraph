// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package json2

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/rpc"
)

type RecordWriter struct {
	Headers      http.Header
	Body         *bytes.Buffer
	ResponseCode int
	Flushed      bool
}

func NewRecordWriter() *RecordWriter {
	return &RecordWriter{
		Headers: make(http.Header),
		Body:    new(bytes.Buffer),
	}
}

func (rw *RecordWriter) Header() http.Header {
	return rw.Headers
}

func (rw *RecordWriter) Write(buf []byte) (int, error) {
	if rw.Body != nil {
		rw.Body.Write(buf)
	}
	if rw.ResponseCode == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	return len(buf), nil
}

func (rw *RecordWriter) WriteHeader(code int) {
	rw.ResponseCode = code
}

func (rw *RecordWriter) Flush() {
	rw.Flushed = true
}

// ------------- Example Service -----------------------

type ServiceRequest struct {
	N int
}

type ServiceResponse struct {
	Result int
}

type Service struct{}

var ErrResponse = errors.New("error response")

func (s *Service) Echo(r *http.Request, req *ServiceRequest, res *ServiceResponse) error {
	log.Printf("ECHO -- Got N: %d", req.N)
	res.Result = req.N
	return nil
}

func (s *Service) Fail(r *http.Request, req *ServiceRequest, res *ServiceResponse) error {
	return ErrResponse
}

// -------------------------------------------------------

func exec(s *rpc.Server, method string, req interface{}, res interface{}) error {
	buf, _ := EncodeClientRequest(method, req)
	body := bytes.NewBuffer(buf)
	r, _ := http.NewRequest("POST", "http://localhost:3000", body)
	r.Header.Set("Content-Type", "application/json")

	w := NewRecordWriter()
	s.ServeHTTP(w, r)

	return DecodeClientResponse(w.Body, res)
}

func execInvalidJSON(s *rpc.Server, res interface{}) error {
	r, _ := http.NewRequest("POST", "http://localhost:3000", strings.NewReader("blahblahblah"))
	r.Header.Set("Content-Type", "application/json")

	w := NewRecordWriter()
	s.ServeHTTP(w, r)

	return DecodeClientResponse(w.Body, res)
}

func TestService(t *testing.T) {
	s := rpc.NewServer()
	s.RegisterCodec(&Codec{})
	err := s.RegisterService(new(Service), "Service")
	if err != nil {
		t.Fatalf("could not register service: %s", err)
	}
	var res ServiceResponse

	// Valid request
	err = exec(s, "Service_Echo", &ServiceRequest{1337}, &res)
	if err != nil {
		t.Fatalf("request execution failed: %s", err)
	}
	if res.Result != 1337 {
		t.Fatalf("response value incorrect. expected: %d got: %d", 10, res.Result)
	}

	// Exepected to return error
	res = ServiceResponse{}
	err = exec(s, "Service_Fail", &ServiceRequest{1337}, &res)
	if err == nil {
		t.Fatalf("expected error to be thrown")
	} else if err.Error() != ErrResponse.Error() {
		t.Fatalf("unexpected error. got: %s expected: %s", err, ErrResponse)
	}

	// Invalid JSON
	res = ServiceResponse{}
	err = execInvalidJSON(s, res)
	if err == nil {
		t.Fatalf("no error thrown from invalid json")
	} else if jsonErr, ok := err.(*Error); !ok {
		t.Fatalf("expected error, got: %s", err)
	} else if jsonErr.ErrorCode != ERR_PARSE {
		t.Fatalf("expected ERR_PARSE (%d), got: %s (%d)", ERR_PARSE, jsonErr.Message, jsonErr.ErrorCode)
	}
}
