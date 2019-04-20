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

package rpc

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"testing"
)

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

// --------------- Mock Codec -----------------------------

type MockCodec struct {
	N int
}

func (c MockCodec) NewRequest(r *http.Request) CodecRequest {
	return MockCodecRequest(c)
}

type MockCodecRequest struct {
	N int
}

func (r MockCodecRequest) Method() (string, error) {
	return "Service.Echo", nil
}

func (r MockCodecRequest) ReadRequest(args interface{}) error {
	req := args.(*ServiceRequest)
	req.N = r.N
	return nil
}

func (r MockCodecRequest) WriteResponse(w http.ResponseWriter, reply interface{}) {
	res := reply.(*ServiceResponse)
	size, err := w.Write([]byte(strconv.Itoa(res.Result)))
	if size != len(strconv.Itoa(res.Result)) {
		log.Printf("expected to write: %d, wrote: %d", len(strconv.Itoa(res.Result)), size)
	}
	if err != nil {
		log.Print(err)
	}
}

func (r MockCodecRequest) WriteError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	size, err := w.Write([]byte(err.Error()))
	if size != len(err.Error()) {
		log.Printf("expected to write: %d, wrote: %d", len(err.Error()), size)
	}
	if err != nil {
		log.Print(err)
	}
}

type MockResponseWriter struct {
	header http.Header
	Status int
	Body   string
}

func NewMockResponseWriter() *MockResponseWriter {
	header := make(http.Header)
	return &MockResponseWriter{header: header}
}

func (w *MockResponseWriter) Header() http.Header {
	return w.header
}

func (w *MockResponseWriter) Write(p []byte) (int, error) {
	w.Body = string(p)
	if w.Status == 0 {
		w.Status = 200
	}
	return len(p), nil
}

func (w *MockResponseWriter) WriteHeader(status int) {
	w.Status = status
}

func TestServeHTTP(t *testing.T) {
	s := NewServer()
	err := s.RegisterService(new(Service), "")
	if err != nil {
		t.Fatal(err)
	}
	s.RegisterCodec(MockCodec{10})

	// Valid request
	r, err := http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	w := NewMockResponseWriter()
	s.ServeHTTP(w, r)
	if w.Status != 200 {
		t.Errorf("unexpected status. got: %d expected: %d", w.Status, 200)
	}
	if w.Body != strconv.Itoa(10) {
		t.Errorf("unexpected body content. got: %s expected %s", w.Body, strconv.Itoa(10))
	}

	// Valid request, multiple content types
	r, err = http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json; some-other-stuff")
	w = NewMockResponseWriter()
	s.ServeHTTP(w, r)
	if w.Status != 200 {
		t.Errorf("unexpected status. got: %d expected: %d", w.Status, 200)
	}
	if w.Body != strconv.Itoa(10) {
		t.Errorf("unexpected body content. got: %s expected %s", w.Body, strconv.Itoa(10))
	}

	// Invalid HTTP method
	r, err = http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	s.ServeHTTP(w, r)
	if w.Status != 405 {
		t.Errorf("unexpected status. got: %d expected: %d", w.Status, 405)
	}

	// Invalid content-type
	r, err = http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "www-url-encoded")
	w = NewMockResponseWriter()
	s.ServeHTTP(w, r)
	if w.Status != 415 {
		t.Errorf("unexpected status. got: %d expected: %d", w.Status, 415)
	}
	if w.Body != strconv.Itoa(10) {
		t.Errorf("unexpected body content. got: %s expected %s", w.Body, strconv.Itoa(10))
	}

	// Invalid content-type
	r, err = http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "www-url-encoded")
	w = NewMockResponseWriter()
	s.ServeHTTP(w, r)
	if w.Status != 415 {
		t.Errorf("unexpected status. got: %d expected: %d", w.Status, 415)
	}
	if w.Body != strconv.Itoa(10) {
		t.Errorf("unexpected body content. got: %s expected %s", w.Body, strconv.Itoa(10))
	}
}
