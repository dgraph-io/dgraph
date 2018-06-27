/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type DgraphCluster struct {
	TokenizerPluginsArg string

	dgraphPort string
	zeroPort   string

	dgraphPortOffset int
	zeroPortOffset   int

	dir    string
	zero   *exec.Cmd
	dgraph *exec.Cmd

	client *dgo.Dgraph
}

func NewDgraphCluster(dir string) *DgraphCluster {
	do := freePort(x.PortGrpc)
	zo := freePort(x.PortZeroGrpc)
	return &DgraphCluster{
		dgraphPort:       strconv.Itoa(do + x.PortGrpc),
		zeroPort:         strconv.Itoa(zo + x.PortZeroGrpc),
		dgraphPortOffset: do,
		zeroPortOffset:   zo,
		dir:              dir,
	}
}

func (d *DgraphCluster) StartZeroOnly() error {
	d.zero = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"zero",
		"-w=wz",
		"-o", strconv.Itoa(d.zeroPortOffset),
		"--replicas", "3",
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
		"--lru_mb=4096",
		"--zero", ":"+d.zeroPort,
		"--port_offset", strconv.Itoa(d.dgraphPortOffset),
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

	d.client = dgo.NewDgraphClient(api.NewDgraphClient(dgConn))

	return nil
}

type Node struct {
	process *exec.Cmd
	offset  string
}

func (d *DgraphCluster) AddNode(dir string) (Node, error) {
	o := strconv.Itoa(freePort(x.PortInternal))
	dgraph := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"server",
		"--lru_mb=4096",
		"--zero", ":"+d.zeroPort,
		"--port_offset", o,
	)
	dgraph.Dir = dir
	dgraph.Stdout = os.Stdout
	dgraph.Stderr = os.Stderr
	x.Check(os.MkdirAll(dir, os.ModePerm))
	err := dgraph.Start()

	return Node{
		process: dgraph,
		offset:  o,
	}, err
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
