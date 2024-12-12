package importP

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/worker"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	Import x.SubCommand
)

type importServer struct {
	pb.ImportPServer
}

var grpcServer *grpc.Server

func (server *importServer) StreamSnapshot(stream pb.ImportP_StreamSnapshotServer) error {
	defer func() {
		go func() {
			fmt.Println("Shutting down gRPC server...")
			grpcServer.GracefulStop()
		}()
	}()
	snap := &pb.Snapshot{}
	snap.SinceTs = 0
	snap.ReadTs = 28

	opt := badger.DefaultOptions("/home/shiva/workspace/dgraph-work/import/p")
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	worker.Pstore = ps

	return worker.DoStreamSnapshot(snap, stream)
}

func init() {
	Import.Cmd = &cobra.Command{
		Use:   "import",
		Short: "Run the import tool",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Import.Cmd.SetHelpTemplate(x.NonRootTemplate)
}

func run() {
	if err := importP(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func importP() error {
	// Create listener for the gRPC server
	lis, err := net.Listen("tcp", "localhost:50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Create gRPC server
	var opts []grpc.ServerOption
	grpcServer = grpc.NewServer(opts...)
	pb.RegisterImportPServer(grpcServer, &importServer{})

	go func() {
		retryCount := 0
		maxRetries := 10
		retryDelay := 2 * time.Second
		for {
			conn, err := grpc.NewClient("localhost:7080", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err == nil {
				defer conn.Close()
				c := pb.NewWorkerClient(conn)

				_, err = c.PDirStat(context.Background(), &pb.PDirReadyStatus{IsReady: true, Ack: true})
				if err == nil {
					fmt.Println("Connected to Dgraph successfully.")
					return
				}
				log.Printf("Failed to send PDirStat: %v", err)
			} else {
				log.Printf("Failed to connect to Dgraph: %v", err)
			}

			retryCount++
			if retryCount >= maxRetries {
				break
			}

			time.Sleep(retryDelay)
		}

	}()

	fmt.Println("Starting gRPC server...")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}

	return nil
}
