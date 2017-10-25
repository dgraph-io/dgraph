package main

import (
	"os"
	"os/exec"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type DgraphCluster struct {
	TokenizerPluginsArg string

	dgraphPort string
	zeroPort   string

	dir    string
	zero   *exec.Cmd
	dgraph *exec.Cmd

	client *client.Dgraph
}

func NewDgraphCluster(dir string) *DgraphCluster {
	return &DgraphCluster{
		dgraphPort: freePort(),
		zeroPort:   freePort(),
		dir:        dir,
	}
}

func (d *DgraphCluster) Start() error {
	d.zero = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraphzero"),
		"-w=wz",
		"-idx=1",
		"-port", d.zeroPort,
		//"-port", freePort(),
		//"-my", ":"+d.zeroPort,
		//"-bindall",
	)
	d.zero.Dir = d.dir
	d.zero.Stdout = os.Stdout
	d.zero.Stderr = os.Stderr
	if err := d.zero.Start(); err != nil {
		return err
	}

	// Wait for dgraphzero to start listening.
	time.Sleep(time.Second * 1)

	d.dgraph = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"-memory_mb=1024",
		"-peer", ":"+d.zeroPort,
		"-port", freePort(),
		"-grpc_port", d.dgraphPort,
		"-workerport", freePort(),
		"-custom_tokenizers", d.TokenizerPluginsArg,
	)
	d.dgraph.Dir = d.dir
	d.dgraph.Stdout = os.Stdout
	d.dgraph.Stderr = os.Stderr
	if err := d.dgraph.Start(); err != nil {
		return err
	}

	dgConn, err := grpc.Dial(":"+d.dgraphPort, grpc.WithInsecure())
	if err != nil {
		return err
	}
	zeroConn, err := grpc.Dial(":"+d.zeroPort, grpc.WithInsecure())
	if err != nil {
		return err
	}

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 4)

	d.client = client.NewDgraphClient(protos.NewZeroClient(zeroConn), protos.NewDgraphClient(dgConn))

	return nil
}

func (d *DgraphCluster) Close() {
	// Ignore errors
	d.zero.Process.Kill()
	d.dgraph.Process.Kill()
}

func (d *DgraphCluster) Query(q string) (string, error) {
	//resp, err := http.Post("http://127.0.0.1:"+d.queryPort+"/query", "", bytes.NewBufferString(q))
	//if err != nil {
	//	return "", err
	//}
	//defer resp.Body.Close()
	//if resp.StatusCode != http.StatusOK {
	//	return "", x.Errorf("Bad status: %v", resp.Status)
	//}
	//body, err := ioutil.ReadAll(resp.Body)
	//if err != nil {
	//	return "", x.Wrapf(err, "could not ready body")
	//}
	//return string(body), nil
	return "", errors.New("not implemented")
}
