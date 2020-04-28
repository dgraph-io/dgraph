package core

import (
	"errors"
	"fmt"
)

// ErrNilBlockState is returned when BlockState is nik
var ErrNilBlockState = errors.New("cannot have nil BlockState")

// ErrNilStorageState is returned when StorageState is nik
var ErrNilStorageState = errors.New("cannot have nil StorageState")

// ErrNilKeystore is returned when keystore is nil
var ErrNilKeystore = errors.New("cannot have nil keystore")

// ErrNoKeysProvided is returned when no keys are given for an authority node
var ErrNoKeysProvided = errors.New("no keys provided for authority node")

// ErrServiceStopped is returned when the service has been stopped
var ErrServiceStopped = errors.New("service has been stopped")

// ErrInvalidBlock is returned when a block cannot be verified
var ErrInvalidBlock = errors.New("could not verify block")

// ErrNilVerifier is returned when trying to instantiate a Syncer without a Verifier
var ErrNilVerifier = errors.New("cannot have nil Verifier")

// ErrNilRuntime is returned when trying to instantiate a Service or Syncer without a runtime
var ErrNilRuntime = errors.New("cannot have nil runtime")

// ErrNilChannel is returned if a channel is nil
func ErrNilChannel(s string) error {
	return fmt.Errorf("cannot have nil channel %s", s)
}

// ErrMessageCast is returned if unable to cast a network.Message to a type
func ErrMessageCast(s string) error {
	return fmt.Errorf("could not cast network.Message to %s", s)
}

// ErrUnsupportedMsgType is returned if we receive an unknown message type
func ErrUnsupportedMsgType(d int) error {
	return fmt.Errorf("received unsupported message type %d", d)
}
