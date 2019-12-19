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
	"context"
	"time"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

// SendStatusInterval is the time between sending status messages
const SendStatusInterval = 5 * time.Minute

// ExpireStatusInterval is the time between expiring status messages
const ExpireStatusInterval = SendStatusInterval + time.Minute

// status submodule
type status struct {
	host          *host
	hostMessage   *StatusMessage
	peerConfirmed map[peer.ID]time.Time
	peerMessage   map[peer.ID]*StatusMessage
}

// newStatus creates a new status instance from host
func newStatus(host *host) (s *status, err error) {
	s = &status{
		host:          host,
		peerConfirmed: make(map[peer.ID]time.Time),
		peerMessage:   make(map[peer.ID]*StatusMessage),
	}
	return s, err
}

// confirmed returns true if peer is confirmed
func (status *status) confirmed(peer peer.ID) bool {
	return !status.peerConfirmed[peer].IsZero()
}

// setHostMessage sets the host status message
func (status *status) setHostMessage(msg Message) {
	status.hostMessage = msg.(*StatusMessage)
}

// handleConn starts status processes upon connection
func (status *status) handleConn(conn network.Conn) {
	ctx := context.Background()
	peer := conn.RemotePeer()

	// check if host message set
	if status.hostMessage != nil {

		// send initial host status message to peer upon connection
		err := status.host.send(peer, status.hostMessage)
		if err != nil {
			log.Error(
				"Failed to send host status message to peer",
				"peer", peer,
				"err", err,
			)
		}

		// handle status message expiration
		go status.expireStatus(ctx, peer)

	} else {
		log.Error(
			"Failed to send host status message to peer",
			"peer", peer,
			"err", "host status message not set",
		)
	}
}

// handleMessage checks if the peer status message is compatibale with the host
// status message, then either manages peer status or closes peer connection
func (status *status) handleMessage(stream network.Stream, msg *StatusMessage) {
	ctx := context.Background()
	peer := stream.Conn().RemotePeer()

	// check if valid status message
	if status.validMessage(msg) {

		// update peer confirmed status message time
		status.peerConfirmed[peer] = time.Now()

		// update peer status message
		status.peerMessage[peer] = msg

		// wait then send next host status message
		go status.sendNextMessage(ctx, peer)

	} else {

		// close connection with peer if status message is not valid
		err := status.closePeer(ctx, peer)
		if err != nil {
			log.Error("Failed to close peer with invalid status message", "err", err)
		}
	}
}

// validMessage confirms the status message is valid
func (status *status) validMessage(msg *StatusMessage) bool {
	switch {
	case msg.GenesisHash != status.hostMessage.GenesisHash:
		log.Error("Failed to validate status message", "err", "genesis hash")
		return false
	case msg.ProtocolVersion < status.hostMessage.MinSupportedVersion:
		log.Error("Failed to validate status message", "err", "protocol version")
		return false
	case msg.MinSupportedVersion > status.hostMessage.ProtocolVersion:
		log.Error("Failed to validate status message", "err", "protocol version")
		return false
	}
	return true
}

// sendNextMessage waits a set time between receiving a valid peer message and
// sending the next host message. The "next" host message is after the initial
// host message sent on connection and all host messages sent through the same
// process; this event should occur at every set time for every connected peer
// using the same 'send on connect' and 'send on receive' protocol). After set
// time, if the peer is still connected, sendNextMessage sends the next host
// message and starts a process that will manage expiratation.
func (status *status) sendNextMessage(ctx context.Context, peer peer.ID) {

	// wait between sending status messages
	time.Sleep(SendStatusInterval)

	// check if peer is still connected
	if status.host.peerConnected(peer) {

		// send host status message to peer
		err := status.host.send(peer, status.hostMessage)
		if err != nil {
			log.Error(
				"Failed to send host status message to peer",
				"peer", peer,
				"err", err,
			)
		}

		// handle status message expiration
		go status.expireStatus(ctx, peer)

	} else {

		// delete peer mappings
		delete(status.peerConfirmed, peer)
		delete(status.peerMessage, peer)

		ctx.Done() // cancel running processes

	}
}

// expireStatus closes peer connection if status message has exipred
func (status *status) expireStatus(ctx context.Context, peer peer.ID) {

	// wait to check for expired status message
	time.Sleep(ExpireStatusInterval)

	// get time of last confirmed status message
	lastConfirmed := status.peerConfirmed[peer]

	// check if status message has expired
	if time.Since(lastConfirmed) > SendStatusInterval {

		// update peer information and close connection
		err := status.closePeer(ctx, peer)
		if err != nil {
			log.Error("Failed to close peer with expired status message", "err", err)
		}
	}
}

// closePeer updates status state and closes the connection
func (status *status) closePeer(ctx context.Context, peer peer.ID) error {

	// cancel running processes
	ctx.Done()

	// delete peer mappings
	delete(status.peerConfirmed, peer)
	delete(status.peerMessage, peer)

	// close connection with peer
	err := status.host.closePeer(peer)

	return err
}
