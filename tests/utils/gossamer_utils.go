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
	"sync"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	log "github.com/ChainSafe/log15"
)

var (
	keyList  = []string{"alice", "bob", "charlie", "dave", "eve", "ferdie", "george", "heather", "ian"}
	basePort = 7000

	// BaseRPCPort is the starting RPC port for test nodes
	BaseRPCPort = 8540

	currentDir, _ = os.Getwd()
	gossamerCMD   = filepath.Join(currentDir, "../..", "bin/gossamer")

	// GenesisOneAuth is the genesis file that has 1 authority
	GenesisOneAuth string = filepath.Join(currentDir, "../utils/genesis_oneauth.json")
	// GenesisThreeAuths is the genesis file that has 3 authorities
	GenesisThreeAuths string = filepath.Join(currentDir, "../utils/genesis_threeauths.json")
	// GenesisDefault is the default gssmr genesis file
	GenesisDefault string = filepath.Join(currentDir, "../..", "chain/gssmr/genesis.json")
)

// Node represents a gossamer process
type Node struct {
	Process  *exec.Cmd
	Key      string
	RPCPort  string
	Idx      int
	basePath string
}

// InitGossamer initializes given node number and returns node reference
func InitGossamer(idx int, basePath, genesis string) (*Node, error) {
	//nolint
	cmdInit := exec.Command(gossamerCMD, "init",
		"--basepath", basePath+strconv.Itoa(idx),
		"--genesis", genesis,
		"--force",
	)

	//add step for init
	log.Info("initializing gossamer...", "cmdInit", cmdInit)
	stdOutInit, err := cmdInit.CombinedOutput()
	if err != nil {
		fmt.Println(stdOutInit)
		return nil, err
	}

	// TODO: get init exit code to see if node was successfully initialized
	log.Info("initialized gossamer!")

	return &Node{
		Idx:      idx,
		RPCPort:  strconv.Itoa(BaseRPCPort + idx),
		basePath: basePath + strconv.Itoa(idx),
	}, nil
}

// StartGossamer starts given node
func StartGossamer(t *testing.T, node *Node) error {
	var key string
	if node.Idx >= len(keyList) {
		//nolint
		node.Process = exec.Command(gossamerCMD, "--port", strconv.Itoa(basePort+node.Idx),
			"--basepath", node.basePath,
			"--rpchost", HOSTNAME,
			"--rpcport", node.RPCPort,
			"--ws=false",
			"--rpcmods", "system,author,chain,state",
			"--roles", "1", // no key provided, non-authority node
			"--rpc",
		)
	} else {
		key = keyList[node.Idx]
		//nolint
		node.Process = exec.Command(gossamerCMD, "--port", strconv.Itoa(basePort+node.Idx),
			"--key", key,
			"--basepath", node.basePath,
			"--rpchost", HOSTNAME,
			"--rpcport", node.RPCPort,
			"--ws=false",
			"--rpcmods", "system,author,chain,state",
			"--roles", "4", // authority node
			"--rpc",
			"--log", "debug",
		)
	}

	node.Key = key

	// a new file will be created, it will be used for log the outputs from the node
	f, err := os.Create(filepath.Join(node.basePath, "gossamer.log"))
	if err != nil {
		log.Error("Error when trying to set a log file for gossamer output", "error", err)
		return err
	}

	//this is required to be able to have multiple inputs into same file
	multiWriter := io.MultiWriter(f, os.Stdout)

	node.Process.Stdout = multiWriter
	node.Process.Stderr = multiWriter

	log.Info("Going to execute gossamer", "cmd", node.Process)
	err = node.Process.Start()
	if err != nil {
		log.Error("Could not execute gossamer cmd", "err", err)
		return err
	}

	log.Info("wait few secs for node to come up", "cmd.Process.Pid", node.Process.Process.Pid)
	var started bool

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if err = CheckNodeStarted(t, "http://"+HOSTNAME+":"+node.RPCPort); err == nil {
			started = true
			break
		} else {
			log.Info("Waiting for Gossamer to start", "err", err)
		}
	}

	if started {
		log.Info("Gossamer started", "key", key, "cmd.Process.Pid", node.Process.Process.Pid)
	} else {
		log.Crit("Gossamer didn't start!", "err", err)
	}

	return nil
}

// RunGossamer will initialize and start a gossamer instance
func RunGossamer(t *testing.T, idx int, basepath, genesis string) (*Node, error) {
	node, err := InitGossamer(idx, basepath, genesis)
	if err != nil {
		log.Crit("could not initialize gossamer", "error", err)
		os.Exit(1)
	}

	err = StartGossamer(t, node)
	if err != nil {
		log.Crit("could not start gossamer", "error", err)
		os.Exit(1)
	}

	return node, nil
}

// CheckNodeStarted check if gossamer node is started
func CheckNodeStarted(t *testing.T, gossamerHost string) error {
	method := "system_health"

	respBody, err := PostRPC(method, gossamerHost, "{}")
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

// InitNodes initializes given number of nodes
func InitNodes(num int) ([]*Node, error) {
	var nodes []*Node
	tempDir, err := ioutil.TempDir("", "gossamer-stress-")
	if err != nil {
		return nil, err
	}

	for i := 0; i < num; i++ {
		node, err := InitGossamer(i, tempDir+strconv.Itoa(i), GenesisDefault)
		if err != nil {
			log.Error("failed to run gossamer", "i", i)
			return nil, err
		}

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// StartNodes starts given array of nodes
func StartNodes(t *testing.T, nodes []*Node) error {
	for _, n := range nodes {
		err := StartGossamer(t, n)
		if err != nil {
			return nil
		}
	}
	return nil
}

// InitializeAndStartNodes will spin up `num` gossamer nodes
func InitializeAndStartNodes(t *testing.T, num int, genesis string) ([]*Node, error) {
	var nodes []*Node

	tempDir, err := ioutil.TempDir("", "gossamer-stress-")
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	wg.Add(num)

	for i := 0; i < num; i++ {
		go func(i int) {
			node, err := RunGossamer(t, i, tempDir+strconv.Itoa(i), genesis)
			if err != nil {
				log.Error("failed to run gossamer", "i", i)
			}

			nodes = append(nodes, node)
			wg.Done()
		}(i)
	}

	wg.Wait()

	return nodes, nil
}

// TearDown will stop gossamer nodes
func TearDown(t *testing.T, nodes []*Node) (errorList []error) {
	for i := range nodes {
		cmd := nodes[i].Process
		err := KillProcess(t, cmd)
		if err != nil {
			log.Error("failed to kill gossamer", "i", i, "cmd", cmd)
			errorList = append(errorList, err)
		}
	}

	return errorList
}
