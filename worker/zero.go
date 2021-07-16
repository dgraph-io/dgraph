/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"context"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// RemoveNodeOverNetwork sends a request to remove the given node from given group to a zero server.
// This operation doesn't necessarily require a zero leader.
func RemoveNodeOverNetwork(ctx context.Context, req *pb.RemoveNodeRequest) (*pb.Status, error) {
	pl := groups().AnyServer(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.RemoveNode(ctx, req)
}

// MoveTabletOverNetwork sends a request to move the given tablet to destination group to the
// current zero leader.
func MoveTabletOverNetwork(ctx context.Context, req *pb.MoveTabletRequest) (*pb.Status, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.MoveTablet(ctx, req)
}

// ApplyLicenseOverNetwork sends a request to apply the given enterprise license to a zero server.
// This operation doesn't necessarily require a zero leader.
func ApplyLicenseOverNetwork(ctx context.Context, req *pb.ApplyLicenseRequest) (*pb.Status, error) {
	pl := groups().AnyServer(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.ApplyLicense(ctx, req)
}
