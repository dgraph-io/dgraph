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

package runtime

import (
	"errors"

	"github.com/gorilla/rpc/v2/json2"
)

// ErrCannotValidateTx is returned if the call to runtime function TaggedTransactionQueueValidateTransaction fails
var ErrCannotValidateTx = errors.New("could not validate transaction")

// ErrInvalidTransaction is returned if the call to runtime function TaggedTransactionQueueValidateTransaction fails with
//  value of [1, 0, x]
var ErrInvalidTransaction = &json2.Error{Code: 1010, Message: "Invalid Transaction"}

// ErrUnknownTransaction is returned if the call to runtime function TaggedTransactionQueueValidateTransaction fails with
//  value of [1, 1, x]
var ErrUnknownTransaction = &json2.Error{Code: 1011, Message: "Unknown Transaction Validity"}

// ErrNilStorage is returned when the runtime context storage isn't set
var ErrNilStorage = errors.New("runtime context storage is nil")
