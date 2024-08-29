package dgraphlite

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgraph/v24/dgraph/cmd"
	"github.com/dgraph-io/dgraph/v24/dgraph/cmd/alpha"
	"github.com/dgraph-io/dgraph/v24/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/x"
)

const (
	schemaQuery        = `schema{}`
	waitDurBeforeRetry = time.Second

	zeroHealthURL     = "http://localhost:6080/health"
	alphaHealthURL    = "http://localhost:8080/health"
	alphaGRPCEndpoint = "localhost:9080"
)

// Dgraph
type Dgraph struct {
	conn *grpc.ClientConn
	dg   *dgo.Dgraph
}

// New returns a new DgraphLite instance. For now, one
// application can only have one DgraphLite instance.
// TODO: Allow multiple instances.
func New(conf Config) (*Dgraph, error) {
	cmd.InitCmds()

	zero.Zero.Conf.Set("wal", path.Join(conf.dataDir, "zw"))
	go zero.Run()

	// By default, ACL is disabled.
	alpha.Alpha.Conf.Set("postings", path.Join(conf.dataDir, "p"))
	alpha.Alpha.Conf.Set("wal", path.Join(conf.dataDir, "w"))
	alpha.Alpha.Conf.Set("tmp", path.Join(conf.dataDir, "t"))
	go alpha.Run()

	// wait for zero & alpha to come up
	if err := healthCheck(zeroHealthURL); err != nil {
		return nil, errors.Wrap(err, "zero not ready")
	}
	if err := healthCheck(alphaHealthURL); err != nil {
		return nil, errors.Wrap(err, "alpha not ready")
	}

	conn, err := grpc.NewClient(alphaGRPCEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to alpha")
	}
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	// wait until we can run queries
	if err := waitUntilReady(dg); err != nil {
		return nil, errors.Wrap(err, "alpha not ready")
	}

	return &Dgraph{conn: conn, dg: dg}, nil
}

// Alter performs schema and predicate operations on the given dgraphlite instance.
func (d *Dgraph) Alter(ctx context.Context, op *api.Operation) error {
	return d.dg.Alter(ctx, op)
}

// Query performs query or mutation thon or upsert on the given dgraphlite instance.
func (d *Dgraph) Query(ctx context.Context, req *api.Request) (*api.Response, error) {
	if len(req.Mutations) > 0 {
		req.CommitNow = true
	}
	return d.dg.NewTxn().Do(ctx, req)
}

// Close closes the Dgraph instance.
func (d *Dgraph) Close() {
	if err := d.conn.Close(); err != nil {
		glog.Warningf("[WARNING] error closing connection: %v", err)
	}

	// stop alpha
	x.ServerCloser.Signal()
	x.ServerCloser.Wait()

	// stop zero
	zero.Stop()
}

func healthCheck(endpoint string) error {
	for i := 0; i < 60; i++ {
		time.Sleep(waitDurBeforeRetry)

		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		body, err := dgraphapi.DoReq(req)
		if err != nil {
			continue
		}
		resp := string(body)

		// zero returns OK in the health check
		if resp == "OK" {
			return nil
		}

		// For Alpha, we always run alpha with EE features enabled
		if !strings.Contains(resp, `"ee_features"`) {
			continue
		}

		return nil
	}

	return fmt.Errorf("health failed, cluster took too long to come up [%v]", endpoint)
}

func waitUntilReady(dg *dgo.Dgraph) error {
	for i := 0; i < 60; i++ {
		time.Sleep(waitDurBeforeRetry)

		if _, err := dg.NewReadOnlyTxn().Query(context.Background(), schemaQuery); err != nil {
			continue
		}

		return nil
	}

	return fmt.Errorf("alpha taking too long to come up")
}
