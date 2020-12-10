/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type DgraphCluster struct {
	TokenizerPluginsArg string

	alphaPort string
	zeroPort  string

	alphaPortOffset int
	zeroPortOffset  int

	dir    string
	zero   *exec.Cmd
	dgraph *exec.Cmd

	client *dgo.Dgraph
}

func NewDgraphCluster(dir string) *DgraphCluster {
	do := freePort(x.PortGrpc)
	zo := freePort(x.PortZeroGrpc)
	return &DgraphCluster{
		alphaPort:       strconv.Itoa(do + x.PortGrpc),
		zeroPort:        strconv.Itoa(zo + x.PortZeroGrpc),
		alphaPortOffset: do,
		zeroPortOffset:  zo,
		dir:             dir,
	}
}

func (d *DgraphCluster) StartZeroOnly() error {
	d.zero = exec.Command(testutil.DgraphBinaryPath(),
		"zero",
		"-w=wz",
		"-o", strconv.Itoa(d.zeroPortOffset),
		"--replicas", "3",
	)
	d.zero.Dir = d.dir
	//d.zero.Stdout = os.Stdout
	//d.zero.Stderr = os.Stderr

	if err := d.zero.Start(); err != nil {
		return err
	}

	// Wait for dgraphzero to start listening and become the leader.
	time.Sleep(time.Second * 6)
	return nil
}

func (d *DgraphCluster) StartAlphaOnly() error {
	d.dgraph = exec.Command(testutil.DgraphBinaryPath(),
		"alpha",
		"--zero", ":"+d.zeroPort,
		"--port_offset", strconv.Itoa(d.alphaPortOffset),
		"--custom_tokenizers", d.TokenizerPluginsArg,
	)
	d.dgraph.Dir = d.dir
	if err := d.dgraph.Start(); err != nil {
		return err
	}

	dgConn, err := grpc.Dial(":"+d.alphaPort, grpc.WithInsecure())
	if err != nil {
		return err
	}

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 6)

	d.client = dgo.NewDgraphClient(api.NewDgraphClient(dgConn))

	return nil
}

func (d *DgraphCluster) Start() error {
	if err := d.StartZeroOnly(); err != nil {
		return err
	}

	return d.StartAlphaOnly()
}

type Node struct {
	process *exec.Cmd
	offset  string
}

func (d *DgraphCluster) AddNode(dir string) (Node, error) {
	o := strconv.Itoa(freePort(x.PortInternal))
	dgraph := exec.Command(testutil.DgraphBinaryPath(),
		"alpha",
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
	if d == nil {
		return
	}
	if d.zero != nil && d.zero.Process != nil {
		d.zero.Process.Kill()
	}
	if d.dgraph != nil && d.dgraph.Process != nil {
		d.dgraph.Process.Kill()
	}
}

type matchExport struct {
	expectedRDF    int
	expectedSchema int
	dir            string
	port           int
}

func matchExportCount(opts matchExport) error {
	// Now try and export data from second server.
	adminUrl := fmt.Sprintf("http://localhost:%d/admin", opts.port)
	params := testutil.GraphQLParams{
		Query: testutil.ExportRequest,
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		return errors.Errorf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(opts.dir, "export"), ".rdf.gz")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("gunzip -c %s | wc -l", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count := strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedRDF) {
		return errors.Errorf("Export count mismatch. Got: %s", count)
	}

	schemaFile, err := findFile(filepath.Join(opts.dir, "export"), ".schema.gz")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("gunzip -c %s | wc -l", schemaFile)
	out, err = exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count = strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedSchema) {
		return errors.Errorf("Schema export count mismatch. Got: %s", count)
	}
	glog.Infoln("Export count matched.")
	return nil
}

func findFile(dir string, ext string) (string, error) {
	var fp string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ext) {
			fp = path
			return nil
		}
		return nil
	})
	return fp, err
}
