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
	"errors"
	"io"
	"math/rand"
)

type clientRequest struct {
	// JSON-RPC version (must be 2.0)
	Version string `json:"jsonrpc"`
	// Service and method name
	Method string `json:"method"`
	// Method params
	Params interface{} `json:"params"`
	// Random request ID
	Id uint64 `json:"id"`
}

type clientResponse struct {
	// JSON-RPC version (must be 2.0)
	Version string `json:"jsonrpc"`
	// Method call resulting value
	Result *json.RawMessage `json:"result"`
	// Error thrown during execution
	Error *json.RawMessage `json:"error"`
}

// EncodeClientRequest marshals struct values for transmission
func EncodeClientRequest(method string, args interface{}) ([]byte, error) {
	c := &clientRequest{
		Version: JSONVersion,
		Method:  method,
		Params:  args,
		Id:      uint64(rand.Int63()),
	}
	return json.Marshal(c)
}

// TODO: Decide how to encode reponse values
// DecodeClientResponse unmarshals the response value
func DecodeClientResponse(r io.Reader, reply interface{}) error {
	var c clientResponse
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		return err
	}
	if c.Error != nil {
		jsonErr := &Error{}
		if err := json.Unmarshal(*c.Error, jsonErr); err != nil {
			jsonErr = &Error{
				ErrorCode: ERR_INTERNAL_ERROR,
				Message:   string(*c.Error),
			}
		}
		return jsonErr
	}

	if c.Result == nil {
		return errors.New("result cannot be null")
	}

	return json.Unmarshal(*c.Result, reply)
}
