package runtime

import (
	"errors"
	"fmt"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/transaction"

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

// ValidateTransaction runs the extrinsic through runtime function TaggedTransactionQueue_validate_transaction and returns *Validity
func (r *Runtime) ValidateTransaction(e types.Extrinsic) (*transaction.Validity, error) {
	ret, err := r.Exec(TaggedTransactionQueueValidateTransaction, e)
	if err != nil {
		return nil, err
	}

	if ret[0] != 0 {
		return nil, determineError(ret)
	}

	v := transaction.NewValidity(0, [][]byte{{}}, [][]byte{{}}, 0, false)
	_, err = scale.Decode(ret[1:], v)

	return v, err
}

func determineError(res []byte) error {
	// confirm we have an error
	if res[0] == 0 {
		return nil
	}

	if res[1] == 0 {
		// transaction is invalid
		return ErrInvalidTransaction
	}

	if res[1] == 1 {
		// transaction validity can't be determined
		return ErrUnknownTransaction
	}

	return ErrCannotValidateTx
}

// SetContext sets the runtime's storage. It should be set before calls to the below functions.
func (r *Runtime) SetContext(s Storage) {
	r.ctx.storage = s
}

// BabeConfiguration gets the configuration data for BABE from the runtime
func (r *Runtime) BabeConfiguration() (*types.BabeConfiguration, error) {
	data, err := r.Exec(BabeAPIConfiguration, []byte{})
	if err != nil {
		return nil, err
	}

	bc := new(types.BabeConfiguration)
	_, err = scale.Decode(data, bc)
	if err != nil {
		return nil, err
	}

	return bc, nil
}

// GrandpaAuthorities returns the genesis authorities from the runtime
func (r *Runtime) GrandpaAuthorities() ([]*types.Authority, error) {
	ret, err := r.Exec(GrandpaAuthorities, []byte{})
	if err != nil {
		return nil, err
	}

	adr, err := scale.Decode(ret, []*types.GrandpaAuthorityDataRaw{})
	if err != nil {
		return nil, err
	}

	return types.GrandpaAuthorityDataRawToAuthorityData(adr.([]*types.GrandpaAuthorityDataRaw))
}

// InitializeBlock calls runtime API function Core_initialize_block
func (r *Runtime) InitializeBlock(header *types.Header) error {
	encodedHeader, err := scale.Encode(header)
	if err != nil {
		return fmt.Errorf("cannot encode header: %s", err)
	}

	encodedHeader = append(encodedHeader, 0)

	_, err = r.Exec(CoreInitializeBlock, encodedHeader)
	return err
}

// InherentExtrinsics calls runtime API function BlockBuilder_inherent_extrinsics
func (r *Runtime) InherentExtrinsics(data []byte) ([]byte, error) {
	return r.Exec(BlockBuilderInherentExtrinsics, data)
}

// ApplyExtrinsic calls runtime API function BlockBuilder_apply_extrinsic
func (r *Runtime) ApplyExtrinsic(data types.Extrinsic) ([]byte, error) {
	return r.Exec(BlockBuilderApplyExtrinsic, data)
}

// FinalizeBlock calls runtime API function BlockBuilder_finalize_block
func (r *Runtime) FinalizeBlock() (*types.Header, error) {
	data, err := r.Exec(BlockBuilderFinalizeBlock, []byte{})
	if err != nil {
		return nil, err
	}

	bh := new(types.Header)
	_, err = scale.Decode(data, bh)
	if err != nil {
		return nil, err
	}

	return bh, nil
}
