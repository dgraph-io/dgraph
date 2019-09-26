// Copyright 2019 ChainSafe Systems (ON) Corp.
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

package main

import (
	"bytes"
	"reflect"

	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/internal/api"
	"github.com/ChainSafe/gossamer/internal/services"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/rpc"

	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/polkadb"
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

func teardown(tempFile *os.File) {
	if err := os.Remove(tempFile.Name()); err != nil {
		log.Warn("cannot create temp file", err)
	}
	if err := os.RemoveAll("./chaingang"); err != nil {
		log.Warn("removal of temp directory bin failed", "err", err)
	}
}

func createTempConfigFile() (*os.File, *cfg.Config) {
	TestDBConfig := &polkadb.Config{
		DataDir: "chaingang",
	}
	TestP2PConfig := &p2p.Config{
		Port:     cfg.DefaultP2PPort,
		RandSeed: cfg.DefaultP2PRandSeed,
		BootstrapNodes: []string{
			"/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"},
	}
	var TestConfig = &cfg.Config{
		P2pCfg: TestP2PConfig,
		DbCfg:  TestDBConfig,
		RpcCfg: cfg.DefaultRpcConfig,
	}
	tmpFile, err := ioutil.TempFile(os.TempDir(), "prefix-")
	if err != nil {
		log.Crit("Cannot create temporary file", err)
		os.Exit(1)
	}

	f := cfg.ToTOML(tmpFile.Name(), TestConfig)
	return f, TestConfig
}

func TestGetConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard

	tc := []struct {
		name     string
		value    string
		usage    string
		expected *cfg.Config
	}{
		{"", "", "", cfg.DefaultConfig},
		{"config", tempFile.Name(), "TOML configuration file", cfgClone},
	}

	for _, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(app, set, nil)

		fig, err := getConfig(context)
		if err != nil {
			teardown(tempFile)
			t.Fatalf("failed to set fig %v", err)
		}

		r := fmt.Sprintf("%+v", fig.RpcCfg)
		rpcExp := fmt.Sprintf("%+v", c.expected.RpcCfg)

		db := fmt.Sprintf("%+v", fig.DbCfg)
		dbExp := fmt.Sprintf("%+v", c.expected.DbCfg)

		peer := fmt.Sprintf("%+v", fig.P2pCfg)
		p2pExp := fmt.Sprintf("%+v", c.expected.P2pCfg)

		if !bytes.Equal([]byte(r), []byte(rpcExp)) {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, r, rpcExp)
		}
		if !bytes.Equal([]byte(db), []byte(dbExp)) {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, db, dbExp)
		}
		if !bytes.Equal([]byte(peer), []byte(p2pExp)) {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, peer, p2pExp)
		}
	}
	defer teardown(tempFile)
}

func TestGetDatabaseDir(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		value    string
		usage    string
		expected string
	}{
		{"", "", "", cfg.DefaultDBConfig.DataDir},
		{"config", tempFile.Name(), "TOML configuration file", "chaingang"},
		{"datadir", "test1", "sets database directory", "test1"},
	}

	for i, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(app, set, nil)
		if i == 0 {
			cfgClone.DbCfg.DataDir = ""
		} else {
			cfgClone.DbCfg.DataDir = "chaingang"
		}
		dir := getDatabaseDir(context, cfgClone)

		if dir != c.expected {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, dir, c.expected)
		}
	}
}

func TestCreateP2PService(t *testing.T) {
	_, cfgClone := createTempConfigFile()
	srv := createP2PService(cfgClone.P2pCfg, nil)

	if srv == nil {
		t.Fatalf("failed to create p2p service")
	}
}

func TestSetBootstrapNodes(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		value    string
		usage    string
		expected []string
	}{
		{"config", tempFile.Name(), "TOML configuration file", cfgClone.P2pCfg.BootstrapNodes},
		{"bootnodes", "test1", "Comma separated enode URLs for P2P discovery bootstrap", []string{"test1"}},
	}

	for i, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(nil, set, nil)

		setBootstrapNodes(context, cfgClone.P2pCfg)

		if cfgClone.P2pCfg.BootstrapNodes[i] != c.expected[0] {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, cfgClone.P2pCfg.BootstrapNodes[i], c.expected)
		}
	}
}

func TestSetRpcModules(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		value    string
		usage    string
		expected []api.Module
	}{
		{"config", tempFile.Name(), "TOML configuration file", []api.Module{"system"}},
		{"rpcmods", "author", "API modules to enable via HTTP-RPC, comma separated list", []api.Module{"author"}},
	}

	for i, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(nil, set, nil)

		setRpcModules(context, cfgClone.RpcCfg)

		if cfgClone.RpcCfg.Modules[i] != c.expected[0] {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, cfgClone.RpcCfg.Modules[i], c.expected)
		}
	}
}

func TestSetRpcHost(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		value    string
		usage    string
		expected string
	}{
		{"", "", "", cfg.DefaultRpcHttpHost},
		{"config", tempFile.Name(), "TOML configuration file", "localhost"},
		{"rpchost", "test1", "HTTP-RPC server listening hostname", "test1"},
	}

	for i, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(nil, set, nil)
		if i == 0 {
			cfgClone.RpcCfg.Host = ""
		} else {
			cfgClone.RpcCfg.Host = "localhost"
		}
		setRpcHost(context, cfgClone.RpcCfg)

		if cfgClone.RpcCfg.Host != c.expected {
			t.Fatalf("test failed: %v, got %+v expected %+v", c.name, cfgClone.RpcCfg.Host, c.expected)
		}
	}
}

func TestStrToMods(t *testing.T) {
	strs := []string{"test1", "test2"}
	mods := strToMods(strs)
	rv := reflect.ValueOf(mods)
	if rv.Kind() == reflect.Ptr {
		t.Fatalf("test failed: got %v expected %v", mods, &[]api.Module{})
	}
}

func TestMakeNode(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		value    string
		usage    string
		expected *cfg.Config
	}{
		{"config", tempFile.Name(), "TOML configuration file", cfgClone},
	}

	for _, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)
		context := cli.NewContext(nil, set, nil)
		d, fig, _ := makeNode(context)
		if reflect.TypeOf(d) != reflect.TypeOf(&dot.Dot{}) {
			t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d), reflect.TypeOf(&dot.Dot{}))
		}
		if reflect.TypeOf(d.Services) != reflect.TypeOf(&services.ServiceRegistry{}) {
			t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d.Services), reflect.TypeOf(&services.ServiceRegistry{}))
		}
		if reflect.TypeOf(d.Rpc) != reflect.TypeOf(&rpc.HttpServer{}) {
			t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d.Rpc), reflect.TypeOf(&rpc.HttpServer{}))
		}
		if reflect.TypeOf(fig) != reflect.TypeOf(&cfg.Config{}) {
			t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(fig), reflect.TypeOf(&cfg.Config{}))
		}
	}
	defer teardown(tempFile)
}

func TestCommands(t *testing.T) {
	tempFile, _ := createTempConfigFile()

	tc := []struct {
		name  string
		value string
		usage string
	}{
		{"config", tempFile.Name(), "TOML configuration file"},
	}

	for _, c := range tc {
		app := cli.NewApp()
		app.Writer = ioutil.Discard
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, c.usage)

		context := cli.NewContext(app, set, nil)
		command := dumpConfigCommand

		err := command.Run(context)
		if err != nil {
			t.Fatalf("should have ran dumpConfig command")
		}
	}
	defer teardown(tempFile)
}
