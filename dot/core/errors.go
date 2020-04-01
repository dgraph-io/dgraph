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

// ErrCannotValidateTx is returned if the call to runtime function TaggedTransactionQueueValidateTransaction fails
var ErrCannotValidateTx = errors.New("could not validate transaction")

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
