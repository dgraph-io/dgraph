package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type DgraphCluster struct {
	TokenizerPluginsArg string

	queryPort string
	grpcPort  string
	dir       string
	zero      *exec.Cmd
	dgraph    *exec.Cmd

	client protos.DgraphClient
}

func NewDgraphCluster(dir string) *DgraphCluster {
	return &DgraphCluster{
		grpcPort:  freePort(),
		queryPort: freePort(),
		dir:       dir,
	}
}

func (d *DgraphCluster) Start() error {
	port := freePort()
	d.zero = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraphzero"), "-w=wz", "-idx=1", "-port", port)
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
		"-peer", ":"+port,
		"-port", d.queryPort,
		"-grpc_port", d.grpcPort,
		"-workerport", freePort(),
		"-custom_tokenizers", d.TokenizerPluginsArg,
	)
	d.dgraph.Dir = d.dir
	d.dgraph.Stdout = os.Stdout
	d.dgraph.Stderr = os.Stderr
	if err := d.dgraph.Start(); err != nil {
		return err
	}

	conn, err := grpc.Dial(d.grpcPort)
	if err != nil {
		return err
	}
	d.client = protos.NewDgraphClient(conn)

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 4)
	return nil
}

func (d *DgraphCluster) Close() {
	// Ignore errors
	d.zero.Process.Kill()
	d.dgraph.Process.Kill()
}

func (d *DgraphCluster) GRPCPort() string {
	return d.grpcPort
}

//func (d *DgraphCluster) DropAll() error {
//}

func (d *DgraphCluster) Query(q string) (string, error) {
	resp, err := http.Post("http://127.0.0.1:"+d.queryPort+"/query", "", bytes.NewBufferString(q))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", x.Errorf("Bad status: %v", resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", x.Wrapf(err, "could not ready body")
	}
	return string(body), nil
}
