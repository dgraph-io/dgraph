package core

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/stretchr/testify/require"
)

func TestProcessBlockRequest(t *testing.T) {
	msgRec := make(chan network.Message)
	msgSend := make(chan network.Message)

	cfg := &Config{
		Keystore:        keystore.NewKeystore(),
		MsgSend:         msgSend,
		MsgRec:          msgRec,
		IsBabeAuthority: false,
	}

	s := newTestService(t, cfg)
	err := s.Start()
	require.Nil(t, err)

	defer func() {
		err = s.Stop()
		require.Nil(t, err)
	}()

	blockAnnounce := &network.BlockAnnounceMessage{
		Number:     big.NewInt(3),
		ParentHash: genesisHeader.Hash(),
	}
	// simulate message sent from network service
	msgRec <- blockAnnounce

	select {
	case msg := <-msgSend:
		msgType := msg.GetType()
		require.Equal(t, network.BlockRequestMsgType, msgType)
	case <-time.After(TestMessageTimeout):
		t.Error("timeout waiting for message")
	}

}

func TestProcessBlockAnnounce(t *testing.T) {
	msgSend := make(chan network.Message)
	newBlocks := make(chan types.Block)

	cfg := &Config{
		MsgSend:         msgSend,
		Keystore:        keystore.NewKeystore(),
		NewBlocks:       newBlocks,
		IsBabeAuthority: false,
	}

	s := newTestService(t, cfg)
	err := s.Start()
	require.Nil(t, err)

	expected := &network.BlockAnnounceMessage{
		Number:         big.NewInt(1),
		ParentHash:     genesisHeader.Hash(),
		StateRoot:      common.Hash{},
		ExtrinsicsRoot: common.Hash{},
		Digest:         nil,
	}

	// simulate block sent from BABE session
	newBlocks <- types.Block{
		Header: &types.Header{
			Number:     big.NewInt(1),
			ParentHash: genesisHeader.Hash(),
		},
		Body: types.NewBody([]byte{}),
	}

	select {
	case msg := <-msgSend:
		msgType := msg.GetType()
		require.Equal(t, network.BlockAnnounceMsgType, msgType)
		require.Equal(t, expected, msg)
	case <-time.After(TestMessageTimeout):
		t.Error("timeout waiting for message")
	}
}
