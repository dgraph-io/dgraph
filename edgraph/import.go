package edgraph

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dgraph-io/badger/v4"
	apiv25 "github.com/dgraph-io/dgo/v240/protos/api.v25"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/golang/glog"
	"google.golang.org/grpc"
)

type Client struct {
	opts grpc.DialOption
	dg   apiv25.DgraphClient
}

func NewImportClient(endpoint string, opts grpc.DialOption) (*Client, error) {
	conn, err := grpc.NewClient(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to endpoint [%s]: %w", endpoint, err)
	}

	return &Client{dg: apiv25.NewDgraphClient(conn), opts: opts}, nil
}

func (c *Client) InitiateSnapshotStream(ctx context.Context) (*apiv25.InitiateSnapshotStreamResponse, error) {
	req := &apiv25.InitiateSnapshotStreamRequest{}
	return c.dg.InitiateSnapshotStream(ctx, req)
}

// StreamSnapshot takes a p directory and a set of group IDs and streams the data from the
// p directory to the corresponding group IDs. The function will skip any groups that do not
// have a corresponding p directory
func (c *Client) StreamSnapshot(ctx context.Context, pDir string, groups []uint32) error {
	groupDirs, err := getPDiPdirrectories(pDir)
	if err != nil {
		return fmt.Errorf("Error getting p directories: %v", err)
	}

	for _, group := range groups {
		fmt.Println("groups are ------------>", group)
		pDir, exists := groupDirs[group-1]
		if !exists {
			fmt.Printf("No p directory found for group %d, skipping...\n", group)
			continue
		}

		if _, err := os.Stat(pDir); os.IsNotExist(err) {
			fmt.Printf("p directory does not exist: %s, skipping...\n", pDir)
			continue
		}

		err = stream(ctx, c.dg, pDir, group)
		if err != nil {
			return err
		}
	}

	return nil
}

func stream(ctx context.Context, dg apiv25.DgraphClient, pdir string, groudId uint32) error {
	out, err := dg.StreamSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to start snapshot stream: %w", err)
	}

	opt := badger.DefaultOptions(pdir)
	ps, err := badger.OpenManaged(opt)
	if err != nil {
		return fmt.Errorf("failed to open BadgerDB: %w", err)
	}
	defer ps.Close()

	// Send group ID first
	groupReq := &apiv25.StreamSnapshotRequest{
		GroupId: groudId,
	}
	if err := out.Send(groupReq); err != nil {
		return fmt.Errorf("Failed to send group ID: %v", err)
	}

	stream := ps.NewStreamAt(math.MaxUint64)
	stream.LogPrefix = "Sending P dir"
	stream.KeyToList = nil
	stream.Send = func(buf *z.Buffer) error {
		kvs := &apiv25.KVS{Data: buf.Bytes()}
		if err := out.Send(&apiv25.StreamSnapshotRequest{
			Kvs: kvs}); err != nil {
			return fmt.Errorf("failed to send data: %w", err)
		}
		return nil
	}

	if err := stream.Orchestrate(ctx); err != nil {
		return fmt.Errorf("stream orchestration failed: %w", err)
	}

	done := &apiv25.KVS{
		Done: true,
	}

	if err := out.Send(&apiv25.StreamSnapshotRequest{
		Kvs: done}); err != nil {
		return fmt.Errorf("failed to send 'done' signal: %w", err)
	}

	ack, err := out.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("failed to receive ACK: %w", err)
	}
	glog.Infof("Received ACK with message: %v\n", ack.Done)

	return nil
}

func getPDiPdirrectories(basePath string) (map[uint32]string, error) {
	groupDirs := make(map[uint32]string)

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			groupID, err := strconv.Atoi(entry.Name())
			if err == nil {
				pDir := filepath.Join(basePath, entry.Name(), "p")
				if _, err := os.Stat(pDir); err == nil {
					groupDirs[uint32(groupID)] = pDir
				}
			}
		}
	}

	return groupDirs, nil
}
