/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package dgraph

import (
	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func GetDefaultEmbeddedConfig() Options {
	config := DefaultConfig
	config.InMemoryComm = true
	config.BaseWorkerPort = 0
	config.MyAddr = ""
	config.PeerAddr = ""

	return config
}

func NewEmbeddedDgraphClient(config Options) *client.Dgraph {

	SetConfiguration(config)

	x.Init(config.DebugMode)
	State = NewServerState()
	schema.Init(State.Pstore)
	posting.Init(State.Pstore)
	worker.Init(State.Pstore)
	worker.StartRaftNodes(State.WALstore, false)

	embedded := &inmemoryClient{&Server{}}
	return client.NewClient([]protos.DgraphClient{embedded})
}

func DisposeEmbeddedDgraph() {
	defer State.Dispose()
	worker.BlockingStop()
}
