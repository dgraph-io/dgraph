/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"github.com/hypermodeinc/dgraph/v24/conn"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
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
