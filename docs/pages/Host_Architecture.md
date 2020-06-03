---
layout: default
title: Host Architecture
permalink: /host-architecture/
---

- _TODO: update "Host Architecture" document [#918](https://github.com/ChainSafe/gossamer/issues/918)_

---

## Nodes

Gossamer includes a base node implementation called the **host node** that implements a shared base protocol for all blockchain protocols within the Polkadot ecosystem. The **host node** is used as the foundation for all **official nodes** within Gossamer and all **custom nodes** built with Gossamer.

### Host Node

The **host node** is the base node implementation. As the base node implementation, the **host node** is not complete without a configuration file, genesis file, compiled runtime, and runtime imports.

### Official Nodes

The **gssmr node** is an official node implementation for the Gossamer Testnet - a configuration file, genesis file, compiled runtime, and runtime imports used with the **host node**.

The **ksmcc node** is an official node implementation for the Kusama Network - a configuration file, genesis file, compiled runtime, and runtime imports used with the **host node**.

The **dotcc node** is an official node implementation for the Polkadot Network - a configuration file, genesis file, compiled runtime, and runtime imports used with the **host node**.

### Custom Services

See [Custom Services](../custom-services/) for more information about building custom node implementations.

## Node Services

The **node services** are the main components of the **host node**:

- **[Core Service](#core-service)**
- **[Network Service](#network-service)**
- **[RPC Service](#rpc-service)**
- **[State Service](#state-service)**

Each **node service** adheres to a common interface:

```go=
type Service interface {
	Start() error
	Stop() error
}
```

- All goroutines within **node services** should start inside `Start`
- All **node services**  can be terminated without consequences by calling `Stop`
- All **node services** whose `Start` method has not been called can be discarded without consequences

### Core Service

The **core service** is responsible for block production and finalization (consensus) and processing messages received from the **network service**; it initializes BABE sessions and GRANDPA rounds and validates blocks and transactions before committing them to the **state service**. 

- only the **core service** writes to block state
- only the **core service** writes to storage state

### Network Service

The **network service** is responsible for coordinating network host and peer interactions. It manages peer connections, receives and parses messages from connected peers and handles each message based on its type. If the message is a non-status message and we have confirmed the status of the connected peer, the message is sent to the **core service** to be processed.

- the **network service** only reads from block state
- only the **network service** writes to network state

#### Host Submodule

The **host submodule** is a wrapper for the libp2p host. This is used to abstract away the details of libp2p and to provide a simple reusable interface for the network host.

```go=
type host struct {
	ctx        context.Context
	h          libp2phost.Host
	dht        *kaddht.IpfsDHT
	bootnodes  []peer.AddrInfo
	protocolID protocol.ID
}
```

#### Status Submodule

The **status submodule** implements the protocol for the status message exchange.

```go=
type status struct {
	host          *host
	hostMessage   *StatusMessage
	peerConfirmed *sync.Map // map of peer.ID to time.Time
	peerMessage   *sync.Map // map of peer.ID to *StatusMessage
}
```

#### Gossip Submodule

The **gossip submodule** stores information about past messages to effectively determine which messages to propagate and which to ignore.

```go=
type gossip struct {
	host    *host
	hasSeen *sync.Map
}
```

#### Syncer Submodule

The **syncer submodule** tracks requested blocks and coordinates syncing with the **core service**.

```go=
type syncer struct {
	host              *host
	blockState        BlockState
	requestedBlockIDs *sync.Map       // track requested block id messages
	syncChan          chan<- *big.Int // chain synchronization channel
}
```

### RPC Service

The **rpc service** is an implementation of the RPC server.

- the **rpc service** only reads from block state
- the **rpc service** only reads from storage state
- the **rpc service** only reads from network state

### State Service

The **state service** is the source of truth for all chain and node state.

- only the **core service** writes to block state
- only the **core service** writes to storage state
- only the **network service** writes to network state

## Node Channels

### Block Channels

_work in progress_

### Message Channels

- `coreMsgs` - messages from the core service
- `networkMsgs` - messages from the network service

within the core service:

- `msgSend` - send messages to the network service
- `msgRec` - receive messages from the network service

within the network service:

- `msgSend` - send messages to the core service
- `msgRec` - receive messages from the core service

### Syncer Channels

_work in progress_
