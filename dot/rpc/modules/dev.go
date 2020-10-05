package modules

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

var blockProducerStoppedMsg = "babe service stopped"
var blockProducerStartedMsg = "babe service started"
var networkStoppedMsg = "network service stopped"
var networkStartedMsg = "network service started"

// DevModule is an RPC module that provides developer endpoints
type DevModule struct {
	networkAPI       NetworkAPI
	blockProducerAPI BlockProducerAPI
}

// NewDevModule creates a new Dev module.
func NewDevModule(bp BlockProducerAPI, net NetworkAPI) *DevModule {
	return &DevModule{
		networkAPI:       net,
		blockProducerAPI: bp,
	}
}

// Control to send start and stop messages to services
func (m *DevModule) Control(r *http.Request, req *[]string, res *string) error {
	reqA := *req
	var err error
	switch reqA[0] {
	case "babe":
		if m.blockProducerAPI == nil {
			return errors.New("not a block producer")
		}

		switch reqA[1] {
		case "stop":
			err = m.blockProducerAPI.Pause()
			*res = blockProducerStoppedMsg
		case "start":
			err = m.blockProducerAPI.Resume()
			*res = blockProducerStartedMsg
		}
	case "network":
		switch reqA[1] {
		case "stop":
			err = m.networkAPI.Stop()
			*res = networkStoppedMsg
		case "start":
			err = m.networkAPI.Start()
			*res = networkStartedMsg
		}
	}
	return err
}

// SetBlockProducerAuthorities dev rpc method that sets authorities for block producer
func (m *DevModule) SetBlockProducerAuthorities(r *http.Request, req *[]interface{}, res *string) error {
	ab := []*types.Authority{}
	for _, v := range *req {
		kb := crypto.PublicAddressToByteArray(common.Address(v.([]interface{})[0].(string)))
		pk, err := sr25519.NewPublicKey(kb)
		if err != nil {
			return err
		}
		bd := &types.Authority{
			Key:    pk,
			Weight: uint64(v.([]interface{})[1].(float64)),
		}
		ab = append(ab, bd)
	}

	err := m.blockProducerAPI.SetAuthorities(ab)
	*res = fmt.Sprintf("set %v block producer authorities", len(ab))
	return err
}

// SetBABEEpochThreshold dev rpc method that sets BABE Epoch Threshold of the BABE Producer
func (m *DevModule) SetBABEEpochThreshold(r *http.Request, req *string, res *string) error {
	n := new(big.Int)
	n, ok := n.SetString(*req, 10)
	if !ok {
		return fmt.Errorf("error setting threshold")
	}
	m.blockProducerAPI.SetThreshold(n)
	*res = fmt.Sprintf("set BABE threshold to %v", n)

	return nil
}

// SetBABERandomness dev rpc method to set BABE Randomness
func (m *DevModule) SetBABERandomness(r *http.Request, req *[]string, res *string) error {
	val := *req

	reqB, err := common.HexToBytes(val[0])
	if err != nil {
		return err
	}

	if len(reqB) != types.RandomnessLength {
		return fmt.Errorf("expected randomness value of %v bytes, received %v bytes", types.RandomnessLength, len(reqB))
	}

	b := [types.RandomnessLength]byte{}
	for i := range b {
		b[i] = reqB[i]
	}
	m.blockProducerAPI.SetRandomness(b)
	*res = "updated BABE Randomness"

	return nil
}
