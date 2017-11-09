package main

import (
	"os"
	"os/exec"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
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

func (d *DgraphCluster) StartZeroOnly() error {
	d.zero = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"zero",
		"-w=wz",
		"--idx=1",
		"--port", d.zeroPort,
	)
	d.zero.Dir = d.dir
	d.zero.Stdout = os.Stdout
	d.zero.Stderr = os.Stderr

	if err := d.zero.Start(); err != nil {
		return err
	}

	// Wait for dgraphzero to start listening and become the leader.
	time.Sleep(time.Second * 4)
	return nil
}

func (d *DgraphCluster) Start() error {
	if err := d.StartZeroOnly(); err != nil {
		return err
	}

	d.dgraph = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"server",
		"--memory_mb=1024",
		"--zero", ":"+d.zeroPort,
		"--port", freePort(),
		"--grpc_port", d.dgraphPort,
		"--workerport", freePort(),
		"--custom_tokenizers", d.TokenizerPluginsArg,
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

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 4)

	d.client = client.NewDgraphClient(protos.NewDgraphClient(dgConn))

	return nil
}

func (d *DgraphCluster) Close() {
	// Ignore errors
	if d.zero != nil && d.zero.Process != nil {
		d.zero.Process.Kill()
	}
	if d.dgraph != nil && d.dgraph.Process != nil {
		d.dgraph.Process.Kill()
	}
}
