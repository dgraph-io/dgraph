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
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
)

// Codec defines the interface for creating a CodecRequest.
type Codec interface {
	NewRequest(*http.Request) CodecRequest
}

// CodecRequest is the interface for a request generated from a codec.
type CodecRequest interface {
	Method() (string, error)
	ReadRequest(interface{}) error
	WriteResponse(http.ResponseWriter, interface{})
	WriteError(w http.ResponseWriter, status int, err error)
}

// Server is an RPC server.
type Server struct {
	codec Codec
	// TODO: need to store content type as well (eg. application/json)
	services *serviceMap
}

// NewServer creates a new Server.
func NewServer() *Server {
	return &Server{
		services: new(serviceMap),
	}
}

// TODO: deal with contentType
// RegisterCodec set the codec for the server.
func (s *Server) RegisterCodec(codec Codec) {
	s.codec = codec
}

// RegisterService adds a service to the servers service map.
func (s *Server) RegisterService(receiver interface{}, name string) error {
	return s.services.register(receiver, name)
}

// ServeHTTP handles http requests to the RPC server.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("Serving HTTP request...")
	if r.Method != "POST" {
		WriteError(w, http.StatusMethodNotAllowed, "rpc: Only accepts POST requests, got: "+r.Method)
	}
	contentType := r.Header.Get("Content-Type")
	idx := strings.Index(contentType, ";")
	if idx != -1 {
		contentType = contentType[:idx]
	}
	if contentType != "application/json" {
		WriteError(w, http.StatusUnsupportedMediaType, "rpc: Only application/json content allowed, got: "+r.Header.Get("Content-Type"))
	}
	log.Println("Got application/json request, proceeding...")
	codecReq := s.codec.NewRequest(r)
	method, errMethod := codecReq.Method()
	if errMethod != nil {
		codecReq.WriteError(w, http.StatusBadRequest, errMethod)
	}
	serviceSpec, methodSpec, errGet := s.services.get(method)
	if errGet != nil {
		codecReq.WriteError(w, http.StatusBadRequest, errGet)
		return
	}

	args := reflect.New(methodSpec.argsType)
	if errRead := codecReq.ReadRequest(args.Interface()); errRead != nil {
		codecReq.WriteError(w, http.StatusBadRequest, errRead)
	}

	reply := reflect.New(methodSpec.replyType)
	errValue := methodSpec.method.Func.Call([]reflect.Value{
		serviceSpec.rcvr,
		reflect.ValueOf(r),
		args,
		reply,
	})

	var errResult error
	statusCode := http.StatusOK
	errInter := errValue[0].Interface()
	if errInter != nil {
		statusCode = http.StatusBadRequest
		errResult = errInter.(error)
	}

	// Encode the response.
	if errResult == nil {
		codecReq.WriteResponse(w, reply.Interface())
	} else {
		codecReq.WriteError(w, statusCode, errResult)
	}
}

// WriteError writes a status and message as the response to a request
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, msg)
}
