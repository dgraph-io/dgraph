package wasmer

import (
	"fmt"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// ValidateTransaction runs the extrinsic through runtime function TaggedTransactionQueue_validate_transaction and returns *Validity
func (r *Instance) ValidateTransaction(e types.Extrinsic) (*transaction.Validity, error) {
	ret, err := r.exec(runtime.TaggedTransactionQueueValidateTransaction, e)
	if err != nil {
		return nil, err
	}

	if ret[0] != 0 {
		return nil, runtime.NewValidateTransactionError(ret)
	}

	v := transaction.NewValidity(0, [][]byte{{}}, [][]byte{{}}, 0, false)
	_, err = scale.Decode(ret[1:], v)

	return v, err
}

// Version calls runtime function Core_Version
func (r *Instance) Version() (*runtime.VersionAPI, error) {
	//TODO ed, change this so that it can lookup runtime by block hash
	version := &runtime.VersionAPI{
		RuntimeVersion: &runtime.Version{},
		API:            nil,
	}

	ret, err := r.exec(runtime.CoreVersion, []byte{})
	if err != nil {
		return nil, err
	}
	err = version.Decode(ret)
	if err != nil {
		return nil, err
	}

	return version, nil
}

// Metadata calls runtime function Metadata_metadata
func (r *Instance) Metadata() ([]byte, error) {
	return r.exec(runtime.Metadata, []byte{})
}

// BabeConfiguration gets the configuration data for BABE from the runtime
func (r *Instance) BabeConfiguration() (*types.BabeConfiguration, error) {
	data, err := r.exec(runtime.BabeAPIConfiguration, []byte{})
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
func (r *Instance) GrandpaAuthorities() ([]*types.Authority, error) {
	ret, err := r.exec(runtime.GrandpaAuthorities, []byte{})
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
func (r *Instance) InitializeBlock(header *types.Header) error {
	encodedHeader, err := scale.Encode(header)
	if err != nil {
		return fmt.Errorf("cannot encode header: %s", err)
	}

	encodedHeader = append(encodedHeader, 0)

	_, err = r.exec(runtime.CoreInitializeBlock, encodedHeader)
	return err
}

// InherentExtrinsics calls runtime API function BlockBuilder_inherent_extrinsics
func (r *Instance) InherentExtrinsics(data []byte) ([]byte, error) {
	return r.exec(runtime.BlockBuilderInherentExtrinsics, data)
}

// ApplyExtrinsic calls runtime API function BlockBuilder_apply_extrinsic
func (r *Instance) ApplyExtrinsic(data types.Extrinsic) ([]byte, error) {
	return r.exec(runtime.BlockBuilderApplyExtrinsic, data)
}

// FinalizeBlock calls runtime API function BlockBuilder_finalize_block
func (r *Instance) FinalizeBlock() (*types.Header, error) {
	data, err := r.exec(runtime.BlockBuilderFinalizeBlock, []byte{})
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

// ExecuteBlock calls runtime function Core_execute_block
func (r *Instance) ExecuteBlock(block *types.Block) ([]byte, error) {
	// copy block since we're going to modify it
	b := block.DeepCopy()

	b.Header.Digest = [][]byte{}
	bdEnc, err := b.Encode()
	if err != nil {
		return nil, err
	}

	return r.exec(runtime.CoreExecuteBlock, bdEnc)
}
