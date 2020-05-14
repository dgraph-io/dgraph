// Copyright 2020 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package utils

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
)

//TODO: #799
var (
	keyList  = []string{"alice", "bob", "charlie", "dave", "eve", "fred", "george", "heather"}
	basePort = 7000

	// BaseRPCPort is the starting RPC port for test nodes
	// It increases by 2 for each node added, since node a port uses --rpcport as the HTTP port and --rpcport + 1 as the websockets port
	BaseRPCPort = 8540
)

// Node represents a gossamer process
type Node struct {
	Process *exec.Cmd
	Key     string
	RPCPort string
	Idx     int
}

// RunGossamer will start a gossamer instance and check if its online and returns CMD, otherwise return err
func RunGossamer(t *testing.T, idx int, dataDir string) (*Node, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	gossamerCMD := filepath.Join(currentDir, "../..", "bin/gossamer")
	genesisPath := filepath.Join(currentDir, "../..", "node/gssmr/genesis.json")

	//nolint
	cmdInit := exec.Command(gossamerCMD, "init",
		"--datadir", dataDir+strconv.Itoa(idx),
		"--genesis", genesisPath,
		"--force",
	)

	//add step for init
	t.Log("Going to init gossamer", "cmdInit", cmdInit)
	stdOutInit, err := cmdInit.CombinedOutput()
	if err != nil {
		t.Error("Could not init gossamer", "err", err, "output", string(stdOutInit))
		return nil, err
	}

	// TODO: get init exit code to see if node was successfully initialized
	t.Log("Gossamer init ok")

	var key string
	var cmd *exec.Cmd
	rpcPort := strconv.Itoa(BaseRPCPort + idx*2) // needs *2 since previous node uses port for rpc and port+1 for WS

	if idx >= len(keyList) {
		//nolint
		cmd = exec.Command(gossamerCMD, "--port", strconv.Itoa(basePort+idx),
			"--datadir", dataDir+strconv.Itoa(idx),
			"--rpchost", HOSTNAME,
			"--rpcport", rpcPort,
			"--rpcmods", "system,author,chain,state",
			"--roles", "1", // no key provided, non-authority node
			"--rpc",
		)
	} else {
		key = keyList[idx]
		//nolint
		cmd = exec.Command(gossamerCMD, "--port", strconv.Itoa(basePort+idx),
			"--key", key,
			"--datadir", dataDir+strconv.Itoa(idx),
			"--rpchost", HOSTNAME,
			"--rpcport", rpcPort,
			"--rpcmods", "system,author,chain,state",
			"--roles", "4", // authority node
			"--rpc",
		)
	}

	// a new file will be created, it will be used for log the outputs from the node
	f, err := os.Create(filepath.Join(dataDir+strconv.Itoa(idx), "gossamer.log"))
	if err != nil {
		t.Fatalf("Error when trying to set a log file for gossamer output: %v", err)
	}

	//this is required to be able to have multiple inputs into same file
	multiWriter := io.MultiWriter(f, os.Stdout)

	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	t.Log("Going to execute gossamer", "cmd", cmd)
	err = cmd.Start()
	if err != nil {
		t.Error("Could not execute gossamer cmd", "err", err)
		return nil, err
	}

	t.Log("wait few secs for node to come up", "cmd.Process.Pid", cmd.Process.Pid)
	var started bool

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if err = CheckNodeStarted(t, "http://"+HOSTNAME+":"+rpcPort); err == nil {
			started = true
			break
		} else {
			t.Log("Waiting for Gossamer to start", "err", err)
		}
	}

	if started {
		t.Log("Gossamer started", "key", key, "cmd.Process.Pid", cmd.Process.Pid)
	} else {
		t.Fatal("Gossamer didn't start!", "err", err)
	}

	return &Node{
		Process: cmd,
		Key:     key,
		RPCPort: rpcPort,
		Idx:     idx,
	}, nil
}

// CheckNodeStarted check if gossamer node is already started
func CheckNodeStarted(t *testing.T, gossamerHost string) error {
	method := "system_health"

	respBody, err := PostRPC(t, method, gossamerHost, "{}")
	if err != nil {
		return err
	}

	target := new(modules.SystemHealthResponse)
	err = DecodeRPC(t, respBody, target)
	if err != nil {
		return err
	}

	if !target.Health.ShouldHavePeers {
		return fmt.Errorf("no peers")
	}

	return nil
}

// KillProcess kills a instance of gossamer
func KillProcess(t *testing.T, cmd *exec.Cmd) error {
	err := cmd.Process.Kill()
	if err != nil {
		t.Log("failed to kill process", "cmd", cmd)
	}
	return err
}

// StartNodes will spin up `num` gossamer nodes
func StartNodes(t *testing.T, num int) ([]*Node, error) {
	var nodes []*Node

	tempDir, err := ioutil.TempDir("", "gossamer-stress-")
	if err != nil {
		t.Log("failed to create tempDir")
		return nil, err
	}

	for i := 0; i < num; i++ {
		node, err := RunGossamer(t, i, tempDir+strconv.Itoa(i))
		if err != nil {
			t.Log("failed to runGossamer", "i", i)
			return nil, err
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// TearDown will stop gossamer nodes
func TearDown(t *testing.T, nodes []*Node) (errorList []error) {
	for i := range nodes {
		cmd := nodes[i].Process
		err := KillProcess(t, cmd)
		if err != nil {
			t.Log("failed to killGossamer", "i", i, "cmd", cmd)
			errorList = append(errorList, err)
		}
	}

	return errorList
}
