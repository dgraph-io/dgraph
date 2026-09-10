/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/v25/conn"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

func forwardAssignUidsToZero(ctx context.Context, in *pb.Num) (*pb.AssignedIds, error) {
	if in.Type != pb.Num_UID {
		return &pb.AssignedIds{}, errors.Errorf("Cannot lease %s via zero proxy", in.Type.String())
	}

	// This is a gRPC entry point, not a continuation of an already-resolved
	// request, so the tenant has to be derived here rather than read off the
	// context. Resolving through the seam means an installed resolver governs this
	// path too, instead of it being hard-wired to the ACL access JWT.
	//
	// The namespace the client sent is cleared first, and that is load-bearing.
	// md["namespace"] is entirely client-controlled server-side, and the built-in
	// resolver leaves it in place when it cannot derive a namespace from the access
	// JWT. Reading it back would mean a caller who omits or corrupts their token
	// leases UIDs in whatever tenant they asked for — which the pre-seam code
	// rejected, because it derived the namespace from the signed JWT and returned
	// that error. Clearing it makes an unattributable request fail here instead.
	if x.MultiTenancyEnabled() {
		rctx, err := x.ResolveTenant(x.ClearIncomingNamespace(ctx))
		if err != nil {
			return &pb.AssignedIds{}, err
		}
		ns, err := x.ExtractNamespace(rctx)
		if err != nil {
			return &pb.AssignedIds{}, err
		}
		ctx = x.AttachNamespaceOutgoing(ctx, ns)
	}

	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	return zc.AssignIds(ctx, in)
}

// RegisterZeroProxyServer forwards select GRPC calls over to Zero
func RegisterZeroProxyServer(s *grpc.Server) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "pb.Zero",
		HandlerType: (*interface{})(nil), // Don't really need complex type checking here
		Methods: []grpc.MethodDesc{
			{
				MethodName: "AssignIds",
				Handler: func(
					srv interface{},
					ctx context.Context,
					dec func(interface{}) error,
					_ grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(pb.Num)
					if err := dec(in); err != nil {
						return nil, err
					}
					return forwardAssignUidsToZero(ctx, in)
				},
			},
		},
	}, &struct{}{})
}
