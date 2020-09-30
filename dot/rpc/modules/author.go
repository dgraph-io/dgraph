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

package modules

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/transaction"
	log "github.com/ChainSafe/log15"
)

// AuthorModule holds a pointer to the API
type AuthorModule struct {
	logger     log.Logger
	coreAPI    CoreAPI
	runtimeAPI RuntimeAPI
	txStateAPI TransactionStateAPI
}

// KeyInsertRequest is used as model for the JSON
type KeyInsertRequest []string

// Extrinsic represents a hex-encoded extrinsic
type Extrinsic string

// ExtrinsicOrHash is a type for Hash and Extrinsic array of bytes
type ExtrinsicOrHash struct {
	Hash      common.Hash
	Extrinsic []byte
}

// ExtrinsicOrHashRequest is a array of ExtrinsicOrHash
type ExtrinsicOrHashRequest []ExtrinsicOrHash

// KeyInsertResponse []byte
type KeyInsertResponse []byte

// PendingExtrinsicsResponse is a bi-dimensional array of bytes for allocating the pending extrisics
type PendingExtrinsicsResponse [][]byte

// RemoveExtrinsicsResponse is a array of hash used to Remove extrinsics
type RemoveExtrinsicsResponse []common.Hash

// KeyRotateResponse is a byte array used to rotate
type KeyRotateResponse []byte

// ExtrinsicStatus holds the actual valid statuses
type ExtrinsicStatus struct {
	IsFuture    bool
	IsReady     bool
	IsFinalized bool
	AsFinalized common.Hash
	IsUsurped   bool
	AsUsurped   common.Hash
	IsBroadcast bool
	AsBroadcast []string
	IsDropped   bool
	IsInvalid   bool
}

// ExtrinsicHashResponse is used as Extrinsic hash response
type ExtrinsicHashResponse string

// NewAuthorModule creates a new Author module.
func NewAuthorModule(logger log.Logger, coreAPI CoreAPI, runtimeAPI RuntimeAPI, txStateAPI TransactionStateAPI) *AuthorModule {
	if logger == nil {
		logger = log.New("service", "RPC", "module", "author")
	}

	return &AuthorModule{
		logger:     logger.New("module", "author"),
		coreAPI:    coreAPI,
		runtimeAPI: runtimeAPI,
		txStateAPI: txStateAPI,
	}
}

// InsertKey inserts a key into the keystore
func (cm *AuthorModule) InsertKey(r *http.Request, req *KeyInsertRequest, res *KeyInsertResponse) error {
	keyReq := *req

	pkDec, err := common.HexToBytes(keyReq[1])
	if err != nil {
		return err
	}

	privateKey, err := keystore.DecodePrivateKey(pkDec, keystore.DetermineKeyType(keyReq[0]))
	if err != nil {
		return err
	}

	keyPair, err := keystore.PrivateKeyToKeypair(privateKey)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(keyPair.Public().Hex(), keyReq[2]) {
		return fmt.Errorf("generated public key does not equal provide public key")
	}

	cm.coreAPI.InsertKey(keyPair)
	cm.logger.Info("inserted key into keystore", "key", keyPair.Public().Hex())
	return nil
}

// HasKey Checks if the keystore has private keys for the given public key and key type.
func (cm *AuthorModule) HasKey(r *http.Request, req *[]string, res *bool) error {
	reqKey := *req
	var err error
	*res, err = cm.coreAPI.HasKey(reqKey[0], reqKey[1])
	return err
}

// PendingExtrinsics Returns all pending extrinsics
func (cm *AuthorModule) PendingExtrinsics(r *http.Request, req *EmptyRequest, res *PendingExtrinsicsResponse) error {
	pending := cm.txStateAPI.Pending()
	resp := [][]byte{}
	for _, tx := range pending {
		enc, err := tx.Encode()
		if err != nil {
			return err
		}
		resp = append(resp, enc)
	}

	*res = PendingExtrinsicsResponse(resp)
	return nil
}

// RemoveExtrinsic Remove given extrinsic from the pool and temporarily ban it to prevent reimporting
func (cm *AuthorModule) RemoveExtrinsic(r *http.Request, req *ExtrinsicOrHashRequest, res *RemoveExtrinsicsResponse) error {
	return nil
}

// RotateKeys Generate new session keys and returns the corresponding public keys
func (cm *AuthorModule) RotateKeys(r *http.Request, req *EmptyRequest, res *KeyRotateResponse) error {
	return nil
}

// SubmitAndWatchExtrinsic Submit and subscribe to watch an extrinsic until unsubscribed
func (cm *AuthorModule) SubmitAndWatchExtrinsic(r *http.Request, req *Extrinsic, res *ExtrinsicStatus) error {
	return nil
}

// SubmitExtrinsic Submit a fully formatted extrinsic for block inclusion
func (cm *AuthorModule) SubmitExtrinsic(r *http.Request, req *Extrinsic, res *ExtrinsicHashResponse) error {
	extBytes, err := common.HexToBytes(string(*req))
	if err != nil {
		return err
	}

	cm.logger.Trace("[rpc]", "extrinsic", extBytes)

	ext := types.Extrinsic(extBytes)
	// validate the transaction
	txv, err := cm.runtimeAPI.ValidateTransaction(ext)
	if err != nil {
		return err
	}

	vtx := transaction.NewValidTransaction(ext, txv)

	if cm.coreAPI.IsBlockProducer() {
		hash := cm.txStateAPI.AddToPool(vtx)
		*res = ExtrinsicHashResponse(hash.String())
		cm.logger.Trace("submitted extrinsic", "tx", vtx, "hash", hash.String())
	}

	//broadcast
	err = cm.coreAPI.HandleSubmittedExtrinsic(ext)
	if err != nil {
		cm.logger.Trace("failed to submit extrinsic to network", "error", err)
	}

	return err
}
