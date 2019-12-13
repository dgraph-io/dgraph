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
// GNU Lesser General Public License for more detailg.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package p2p

import (
	"time"

	"github.com/ChainSafe/gossamer/common"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

// SendStatusInterval is the time between sending status messages
const SendStatusInterval = 5 * time.Minute

// TODO: generate host status message
var hostStatusMessage = &StatusMessage{
	ProtocolVersion:     0,
	MinSupportedVersion: 0,
	Roles:               0,
	BestBlockNumber:     0,
	BestBlockHash:       common.Hash{0x00},
	GenesisHash:         common.Hash{0x00},
	ChainStatus:         []byte{0},
}

// status submodule
type status struct {
	host          *host
	peerConfirmed map[peer.ID]bool
	peerInfo      map[peer.ID]PeerInfo
}

// newStatus creates a new status submodule
func newStatus(host *host) (s *status, err error) {
	s = &status{
		host:          host,
		peerConfirmed: make(map[peer.ID]bool),
		peerInfo:      make(map[peer.ID]PeerInfo),
	}
	return s, err
}

// handleConn starts status processes upon connection
func (s *status) handleConn(conn network.Conn) {

	// starts sending status messages to connected peer
	go s.sendMessages(conn.RemotePeer())

}

// sendMessages sends status messages to the connected peer
func (status *status) sendMessages(peer peer.ID) {
	for {
		// TODO: use generated status message
		msg := hostStatusMessage

		// send host status message to peer
		err := status.host.send(peer, msg)
		if err != nil {
			log.Error(
				"Failed to send status message to peer",
				"peer", peer,
				"err", err,
			)
		}

		// wait between sending status messages
		time.Sleep(SendStatusInterval)
	}
}

// handleMessage checks if the peer status is compatibale with the host status,
// then updates peer confirmation, then updates peer info or drops the peer
func (status *status) handleMessage(stream network.Stream, msg *StatusMessage) {
	peer := stream.Conn().RemotePeer()

	// check if valid status message
	if status.validMessage(msg) {

		// update peer confirmed to true
		status.peerConfirmed[peer] = true

		// update peer network information
		status.peerInfo[peer] = PeerInfo{
			PeerId:          peer.String(),
			Roles:           msg.Roles,
			ProtocolVersion: msg.ProtocolVersion,
			BestHash:        msg.BestBlockHash,
			BestNumber:      msg.BestBlockNumber,
		}

		// TODO: unset peer confirmed after some time

	} else {

		// ensure peer confirmed is false
		status.peerConfirmed[peer] = false

		// close connection with peer if status message is not valid
		err := stream.Conn().Close()
		if err != nil {
			log.Error("Failed to close peer with invalid status message", "err", err)
		}
	}
}

// validMessage confirms the status message is valid
func (status *status) validMessage(msg *StatusMessage) bool {

	// TODO: generate host status message
	hostStatusMessage := hostStatusMessage

	// TODO: implement status message confirmation
	return hostStatusMessage.String() == msg.String()
}
