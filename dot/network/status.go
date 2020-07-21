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

package network

import (
	"bytes"
	"context"
	"sync"
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
	logger        log.Logger
	host          *host
	hostMessage   *StatusMessage
	peerConfirmed *sync.Map //map of peer.ID to time.Time
	peerMessage   *sync.Map //map of peer.ID to *StatusMessage
}

// newStatus creates a new status instance from host
func newStatus(host *host) *status {
	return &status{
		logger:        host.logger.New("module", "status"),
		host:          host,
		peerConfirmed: &sync.Map{},
		peerMessage:   &sync.Map{},
	}
}

// confirmed returns true if peer is confirmed
func (status *status) confirmed(peer peer.ID) bool {
	if t, ok := status.peerConfirmed.Load(peer); ok {
		return !t.(time.Time).IsZero()
	}

	return false
}

// setHostMessage sets the host status message
func (status *status) setHostMessage(msg Message) {
	status.hostMessage = msg.(*StatusMessage)
}

// handleConn starts status processes upon connection
func (status *status) handleConn(conn network.Conn) {
	ctx := context.Background()
	remotePeer := conn.RemotePeer()

	// check if host message set
	if status.hostMessage != nil {

		// send initial host status message to peer upon connection
		err := status.host.send(remotePeer, "", status.hostMessage)
		if err != nil {
			status.logger.Error(
				"Failed to send status message to peer",
				"peer", remotePeer,
				"error", err,
			)
		}

		// handle status message expiration
		go status.expireStatus(ctx, remotePeer)

	} else {
		status.logger.Error(
			"Failed to send status message to peer",
			"peer", remotePeer,
			"error", "host status message not set",
		)
	}
}

// handleMessage checks if the peer status message is compatible with the host
// status message, then either manages peer status or closes peer connection
func (status *status) handleMessage(peer peer.ID, msg *StatusMessage) {
	ctx := context.Background()

	// check if valid status message
	if status.validMessage(msg) {

		// update peer confirmed status message time
		status.peerConfirmed.Store(peer, time.Now())

		// update peer status message
		status.peerMessage.Store(peer, msg)

		// wait then send next host status message
		go status.sendNextMessage(ctx, peer)

	} else {

		// close connection with peer if status message is not valid
		err := status.closePeer(ctx, peer)
		if err != nil {
			status.logger.Error("Failed to close peer with invalid status message", "error", err)
		}
	}
}

// validMessage confirms the status message is valid
func (status *status) validMessage(msg *StatusMessage) bool {
	if status.hostMessage == nil {
		return false
	}

	status.logger.Debug("Validating peer status message", "GenesisHash", msg.GenesisHash)

	switch {
	case !bytes.Equal(msg.GenesisHash[:], status.hostMessage.GenesisHash[:]):
		status.logger.Error(
			"Failed to validate status message",
			"error", "host and peer genesis hashes do not match",
		)
		return false
	case msg.ProtocolVersion < status.hostMessage.MinSupportedVersion:
		status.logger.Error(
			"Failed to validate status message",
			"error", "protocol version less than minimum supported version",
		)
		return false
	case msg.MinSupportedVersion > status.hostMessage.ProtocolVersion:
		status.logger.Error(
			"Failed to validate status message",
			"error", "minimum supported version greater than protocol version",
		)
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
		err := status.host.send(peer, "", status.hostMessage)
		if err != nil {
			status.logger.Error(
				"Failed to send host status message to peer",
				"peer", peer,
				"error", err,
			)
		}

		// handle status message expiration
		go status.expireStatus(ctx, peer)

	} else {

		// delete peer mappings
		status.peerConfirmed.Delete(peer)
		status.peerMessage.Delete(peer)

		ctx.Done() // cancel running processes

	}
}

// expireStatus closes peer connection if status message has exipred
func (status *status) expireStatus(ctx context.Context, peer peer.ID) {

	// wait to check for expired status message
	// TODO: this wakes up every 6 minutes, while sendNextMessage wakes up every 5 minutes.
	// this may cause the messages to get out of sync, investigate
	time.Sleep(ExpireStatusInterval)

	// get time of last confirmed status message
	lastConfirmed, ok := status.peerConfirmed.Load(peer)
	if !ok {
		return
	}

	// check if status message has expired
	if time.Since(lastConfirmed.(time.Time)) > SendStatusInterval {

		// update peer information and close connection
		err := status.closePeer(ctx, peer)
		if err != nil {
			status.logger.Error("Failed to close peer with expired status message", "error", err)
		}
	}
}

// closePeer updates status state and closes the connection
func (status *status) closePeer(ctx context.Context, peer peer.ID) error {

	// cancel running processes
	ctx.Done()

	// delete peer mappings
	status.peerConfirmed.Delete(peer)
	status.peerMessage.Delete(peer)

	// close connection with peer
	err := status.host.closePeer(peer)

	return err
}
