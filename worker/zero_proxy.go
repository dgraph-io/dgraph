package worker

import (
	"context"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"google.golang.org/grpc"
)

func forwardAssignUidsToZero(ctx context.Context, in *pb.Num) (*pb.AssignedIds, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	return zc.AssignUids(ctx, in)
}

// RegisterZeroProxyServer forwards select GRPC calls over to Zero
func RegisterZeroProxyServer(s *grpc.Server) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "pb.Zero",
		HandlerType: (*interface{})(nil), // Don't really need complex type checking here
		Methods: []grpc.MethodDesc{
			{
				MethodName: "AssignUids",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
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
