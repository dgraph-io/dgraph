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

type ErrCode int

// Offical JSON-RPC 2.0 Error codes
// https://www.jsonrpc.org/specification#error_object
const (
	ERR_PARSE          ErrCode = -32700
	ERR_INVALID_REQ    ErrCode = -32600
	ERR_INVALID_METHOD ErrCode = -32601
	ERR_INVALID_PARAMS ErrCode = -32602
	ERR_INTERNAL_ERROR ErrCode = -32603
	// -32000 to -32099 are reserved for implementation specific errors
)

type Error struct {
	Message   string
	ErrorCode ErrCode
}

func (e *Error) Error() string {
	return e.Message
}
