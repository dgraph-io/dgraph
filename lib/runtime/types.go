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

// Version struct
type Version struct {
	Spec_name         []byte
	Impl_name         []byte
	Authoring_version int32
	Spec_version      int32
	Impl_version      int32
}

var (
	// CoreVersion returns the string representing
	CoreVersion = "Core_version"
	// CoreInitializeBlock returns the string representing
	CoreInitializeBlock = "Core_initialize_block"
	// CoreExecuteBlock returns the string representing
	CoreExecuteBlock = "Core_execute_block"
	// TaggedTransactionQueueValidateTransaction returns the string representing
	TaggedTransactionQueueValidateTransaction = "TaggedTransactionQueue_validate_transaction"
	// AuraAPIAuthorities represents the AuraApi_authorities call
	AuraAPIAuthorities = "AuraApi_authorities" // TODO: deprecated with newest runtime, should be Grandpa_authorities
	// BabeAPIConfiguration returns the string representing
	BabeAPIConfiguration = "BabeApi_configuration"
	// BlockBuilderInherentExtrinsics returns the string representing
	BlockBuilderInherentExtrinsics = "BlockBuilder_inherent_extrinsics"
	// BlockBuilderApplyExtrinsic returns the string representing
	BlockBuilderApplyExtrinsic = "BlockBuilder_apply_extrinsic"
	// BlockBuilderFinalizeBlock returns the string representing
	BlockBuilderFinalizeBlock = "BlockBuilder_finalize_block"
)
