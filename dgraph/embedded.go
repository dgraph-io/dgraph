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
	"github.com/dgraph-io/dgraph/protos"
)

// TODO(tzdybal) - server configuration
func NewEmbeddedDgraphClient(opts client.BatchMutationOptions) *client.Dgraph {
	// TODO(tzdybal) - create and setup embedded server backend
	// TODO(tzdybal) - force exactly one group. And don't open up Grpc conns for worker.
	embedded := &inmemoryClient{&Server{}}

	return client.NewClient([]protos.DgraphClient{embedded}, opts)
}
