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
	"encoding/json"
	"log"
	"net/http"

	"github.com/ChainSafe/gossamer/rpc"
)

var JSONVersion = "2.0"

type serverRequest struct {
	// JSON-RPC Version
	Version string `json:"jsonrpc"`
	// Method name
	Method string `json:"method"`
	// Method params
	Params *json.RawMessage `json:"params"`
	// Request id, may be int or string
	Id *json.RawMessage `json:"id"`
}

type serverResponse struct {
	// JSON-RPC Version
	Version string `json:"jsonrpc"`
	// Resulting values
	Result interface{} `json:"result"`
	// Any generated errors
	Error *Error `json:"error"`
	// Request id
	Id *json.RawMessage `json:"id"`
}

// Codec is used to define the JSON codec methods to adhere to the Codec interface inside the rpc package.
type Codec struct{}

// NewRequest intercepts a request and parses it into a rpc.CodecRequest
func (c *Codec) NewRequest(r *http.Request) rpc.CodecRequest {
	req := new(serverRequest)
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		err = &Error{
			ErrorCode: ERR_PARSE,
			Message:   err.Error(),
		}
	} else if req.Version != JSONVersion {
		err = &Error{
			ErrorCode: ERR_PARSE,
			Message:   "must be JSON-RPC version " + JSONVersion,
		}
	}

	errClose := r.Body.Close()
	if errClose != nil {
		log.Fatal(errClose)
	}
	return &CodecRequest{request: req, err: err}
}

// CodecRequest is used to store a server request and any related errors for the codec to process.
type CodecRequest struct {
	request *serverRequest
	err     error
}

// Method returns the service and method name from the request.
func (c *CodecRequest) Method() (string, error) {
	if c.err == nil {
		return c.request.Method, nil
		// TODO: Modify methods to match Go naming conventions
		//return strings.Title(strings.Replace(c.request.Method, "_", ".", 1)), nil

	}
	return "", c.err
}

// ReadRequest parses the handler params
func (c *CodecRequest) ReadRequest(args interface{}) error {
	// TODO: Check if params is nil?
	if c.err == nil {
		err := json.Unmarshal(*c.request.Params, args)
		if err != nil {
			c.err = &Error{
				Message:   err.Error(),
				ErrorCode: ERR_PARSE,
			}
		}
	}
	return c.err
}

// WriteResponse creates a serverResponse and passes it to the encoder
func (c *CodecRequest) WriteResponse(w http.ResponseWriter, reply interface{}) {
	res := &serverResponse{
		Version: JSONVersion,
		Result:  reply,
		Id:      c.request.Id,
	}
	c.writeServerResponse(w, res)
}

// WriteError attempts to format err and pass it to the encoder
func (c *CodecRequest) WriteError(w http.ResponseWriter, status int, err error) {
	jsonErr, ok := err.(*Error)
	if !ok {
		jsonErr = &Error{
			ErrorCode: ERR_INTERNAL_ERROR,
			Message:   err.Error(),
		}
	}
	res := &serverResponse{
		Version: JSONVersion,
		Error:   jsonErr,
		Id:      c.request.Id,
	}
	c.writeServerResponse(w, res)
}

// writeServerResponse sets the response header and encodes the response
func (c *CodecRequest) writeServerResponse(w http.ResponseWriter, res *serverResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	err := encoder.Encode(res)
	if err != nil {
		log.Print(err)
	}
}

type EmptyResponse struct{}
