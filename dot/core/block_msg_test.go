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

func TestProcessBlockRequestAndBlockAnnounce(t *testing.T) {
	testCases := []struct {
		name          string
		blockAnnounce *network.BlockAnnounceMessage
		msgType       int
		msgTypeString string
	}{
		{
			name: "should respond with a BlockRequestMessage",
			blockAnnounce: &network.BlockAnnounceMessage{
				Number:         big.NewInt(1),
				ParentHash:     common.Hash{},
				StateRoot:      common.Hash{},
				ExtrinsicsRoot: common.Hash{},
				Digest:         [][]byte{},
			},
			msgType:       network.BlockRequestMsgType, //1
			msgTypeString: "BlockRequestMsgType",
		},
		{
			name: "should respond with a BlockAnnounceMessage",
			blockAnnounce: &network.BlockAnnounceMessage{
				Number:         big.NewInt(2),
				ParentHash:     common.Hash{},
				StateRoot:      common.Hash{},
				ExtrinsicsRoot: common.Hash{},
				Digest:         [][]byte{},
			},
			msgType:       network.BlockAnnounceMsgType, //3
			msgTypeString: "BlockAnnounceMsgType",
		},
	}

	for _, test := range testCases {

		localTest := test
		t.Run(test.name, func(t *testing.T) {
			msgRec := make(chan network.Message)
			msgSend := make(chan network.Message)
			newBlocks := make(chan types.Block)

			// Create header
			header0 := &types.Header{
				Number:     big.NewInt(1),
				ParentHash: genesisHeader.Hash(),
			}

			// BlockBody with fake extrinsics
			blockBody0 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

			block0 := &types.Block{
				Header: header0,
				Body:   &blockBody0,
			}

			cfg := &Config{
				MsgSend:   msgSend,
				Keystore:  keystore.NewKeystore(),
				NewBlocks: newBlocks,
			}

			if localTest.msgType == network.BlockRequestMsgType {
				cfg.IsBabeAuthority = false
				cfg.NewBlocks = nil
				cfg.MsgRec = msgRec
			}

			s, dbSrv := newTestService(t, cfg)
			defer func() {
				err := dbSrv.Stop()
				if err != nil {
					t.Fatal(err)
				}
			}()

			err := s.Start()
			require.Nil(t, err)

			if localTest.msgType == network.BlockAnnounceMsgType {
				// Add the block0 to the DB
				err = s.blockState.AddBlock(block0)
				require.Nil(t, err)
			}

			defer func() {
				err = s.Stop()
				require.Nil(t, err)
			}()

			if localTest.msgType == network.BlockAnnounceMsgType {
				// simulate block sent from BABE session
				newBlocks <- types.Block{
					Header: &types.Header{
						Number:     big.NewInt(2),
						ParentHash: header0.Hash(),
					},
					Body: types.NewBody([]byte{}),
				}
			} else if localTest.msgType == network.BlockRequestMsgType {
				blockAnnounce := &network.BlockAnnounceMessage{
					Number:     big.NewInt(2),
					ParentHash: header0.Hash(),
				}
				// simulate message sent from network service
				msgRec <- blockAnnounce
			}

			select {
			case msg := <-msgSend:
				msgType := msg.GetType()
				require.Equal(t, localTest.msgType, msgType)
			case <-time.After(TestMessageTimeout):
				t.Error("timeout waiting for message")
			}
		})
	}
}
