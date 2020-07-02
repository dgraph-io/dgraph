// Copyright 2020 ChainSafe Systems (ON) Corp.
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
package extrinsic

import (
	"math/big"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// Pallet index for module extrinsic is calling
type Pallet byte

// consts for node_runtime Pallets
const (
	System Pallet = iota
	Utility
	Babe
	Timestamp
	Authorship
	Indices
	Balances
	Staking
	Session
)

// PalletFunction for function index within pallet
type PalletFunction byte

// pallet_balances function index
const (
	PB_Transfer PalletFunction = iota
	PB_Set_balance
	PB_Force_transfer
	PB_Transfer_keep_alive
)

// pallet_system function index
const (
	SYS_fill_block PalletFunction = iota
	SYS_remark
	SYS_set_heap_pages
	SYS_set_code
	SYS_set_storage
	SYS_kill_storage
	SYS_kill_prefix
)

// session function index
const (
	SESS_set_keys PalletFunction = iota
)

// Function struct to represent extrinsic call function
type Function struct {
	Pall         Pallet
	PallFunc     PalletFunction
	FuncCallData interface{}
}

// Signature struct to represent signature parts
type Signature struct {
	Address []byte
	Sig     []byte
	Extra   []byte
}

// UncheckedExtrinsic generic implementation of pre-verification extrinsic
type UncheckedExtrinsic struct {
	Signature Signature
	Function  Function
}

// CreateUncheckedExtrinsic builds UncheckedExtrinsic given function interface, index, genesisHash and Keypair
func CreateUncheckedExtrinsic(fnc *Function, index *big.Int, signer crypto.Keypair, additional interface{}) (*UncheckedExtrinsic, error) {
	extra := struct {
		Era                      [1]byte // TODO determine how Era is determined (Immortal is [1]byte{0}, Mortal is [2]byte{X, 0}, Need to determine how X is calculated)
		Nonce                    *big.Int
		ChargeTransactionPayment *big.Int
	}{
		[1]byte{0},
		index,
		big.NewInt(0),
	}

	payload := buildPayload(fnc, extra, additional)
	payloadEnc, err := payload.Encode()
	if err != nil {
		return nil, err
	}

	signedPayload, err := signer.Sign(payloadEnc)
	if err != nil {
		return nil, err
	}

	extraEnc, err := scale.Encode(extra)
	if err != nil {
		return nil, err
	}
	// TODO this changes mortality, determine how to set
	//extraEnc = append([]byte{22}, extraEnc...)

	signature := Signature{
		Address: signer.Public().Encode(),
		Sig:     signedPayload,
		Extra:   extraEnc,
	}
	ux := &UncheckedExtrinsic{
		Function:  *fnc,
		Signature: signature,
	}
	return ux, nil
}

// CreateUncheckedExtrinsicUnsigned to build unsigned extrinsic
func CreateUncheckedExtrinsicUnsigned(fnc *Function) (*UncheckedExtrinsic, error) {
	ux := &UncheckedExtrinsic{
		Function: *fnc,
	}
	return ux, nil
}

// Encode scale encode UncheckedExtrinsic
func (ux *UncheckedExtrinsic) Encode() ([]byte, error) {
	// TODO deal with signed/unsigned encoding
	enc := []byte{}
	sigEnc, err := ux.Signature.Encode()
	if err != nil {
		return nil, err
	}
	enc = append(enc, sigEnc...)

	fncEnc, err := ux.Function.Encode()
	if err != nil {
		return nil, err
	}
	enc = append(enc, fncEnc...)

	sEnc, err := scale.Encode(enc)
	if err != nil {
		return nil, err
	}

	return sEnc, nil
}

// Encode to encode Signature type
func (s *Signature) Encode() ([]byte, error) {
	enc := []byte{}
	//TODO determine why this 255 byte is here
	addEnc, err := scale.Encode(append([]byte{255}, s.Address...))
	if err != nil {
		return nil, err
	}
	enc = append(enc, addEnc...)
	// TODO find better way to handle keytype
	enc = append(enc, []byte{1}...) //this seems to represent signing key type 0 - Ed25519, 1 - Sr22219, 2 - Ecdsa
	enc = append(enc, s.Sig...)
	enc = append(enc, s.Extra...)
	return enc, nil
}

// Encode scale encode the UncheckedExtrinsic
func (f *Function) Encode() ([]byte, error) {
	enc := []byte{byte(f.Pall), byte(f.PallFunc)}
	dataEnc, err := scale.Encode(f.FuncCallData)
	if err != nil {
		return nil, err
	}
	return append(enc, dataEnc...), nil
}

// payload struct to hold items that need to be signed
type payload struct {
	Function       Function
	Extra          interface{}
	AdditionSigned interface{}
}

func buildPayload(fnc *Function, extra interface{}, additional interface{}) payload {
	return payload{
		Function:       *fnc,
		Extra:          extra,
		AdditionSigned: additional,
	}
}

// Encode scale encode SignedPayload
func (sp *payload) Encode() ([]byte, error) {
	enc, err := sp.Function.Encode()
	if err != nil {
		return nil, err
	}

	exEnc, err := scale.Encode(sp.Extra)
	if err != nil {
		return nil, err
	}

	// TODO this changes mortality, determine how to set
	//exEnc = append([]byte{22}, exEnc...)  // testing era
	enc = append(enc, exEnc...)

	addEnc, err := scale.Encode(sp.AdditionSigned)
	if err != nil {
		return nil, err
	}
	enc = append(enc, addEnc...)

	return enc, nil
}
