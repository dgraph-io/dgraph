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

package sync

import (
	"errors"
	"fmt"
)

// ErrNilBlockState is returned when BlockState is nil
var ErrNilBlockState = errors.New("cannot have nil BlockState")

// ErrNilStorageState is returned when StorageState is nil
var ErrNilStorageState = errors.New("cannot have nil StorageState")

// ErrNilVerifier is returned when trying to instantiate a Syncer without a Verifier
var ErrNilVerifier = errors.New("cannot have nil Verifier")

// ErrNilRuntime is returned when trying to instantiate a Service or Syncer without a runtime
var ErrNilRuntime = errors.New("cannot have nil runtime")

// ErrServiceStopped is returned when the service has been stopped
var ErrServiceStopped = errors.New("service has been stopped")

// ErrInvalidBlock is returned when a block cannot be verified
var ErrInvalidBlock = errors.New("could not verify block")

// ErrNilChannel is returned if a channel is nil
func ErrNilChannel(s string) error {
	return fmt.Errorf("cannot have nil channel %s", s)
}
