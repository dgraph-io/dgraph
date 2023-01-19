package worker

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func forwardAssignUidsToZero(ctx context.Context, in *pb.Num) (*pb.AssignedIds, error) {
	if in.Type != pb.Num_UID {
		return &pb.AssignedIds{}, errors.Errorf("Cannot lease %s via zero proxy", in.Type.String())
	}

	if x.WorkerConfig.AclEnabled {
		var err error
		ctx, err = x.AttachJWTNamespaceOutgoing(ctx)
		if err != nil {
			return &pb.AssignedIds{}, err
		}
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
