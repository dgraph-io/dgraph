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
	"path/filepath"
	"reflect"

	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/internal/api"
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

const TestDataDir = "./test_data"

func teardown(tempFile *os.File) {
	if err := os.Remove(tempFile.Name()); err != nil {
		log.Warn("cannot remove temp file", "err", err)
	}
}

func removeTestDataDir() {
	if err := os.RemoveAll(TestDataDir); err != nil {
		log.Warn("cannot remove test data dir", "err", err)
	}
}

func createTempConfigFile() (*os.File, *cfg.Config) {
	testConfig := cfg.DefaultConfig()
	testConfig.Global.DataDir = TestDataDir

	tmpFile, err := ioutil.TempFile(os.TempDir(), "prefix-")
	if err != nil {
		log.Crit("Cannot create temporary file", "err", err)
		os.Exit(1)
	}

	f := cfg.ToTOML(tmpFile.Name(), testConfig)
	return f, testConfig
}

// Creates a cli context for a test given a set of flags and values
func createCliContext(description string, flags []string, values []interface{}) (*cli.Context, error) {
	set := flag.NewFlagSet(description, 0)
	for i := range values {
		switch v := values[i].(type) {
		case bool:
			set.Bool(flags[i], v, "")
		case string:
			set.String(flags[i], v, "")
		case uint:
			set.Uint(flags[i], v, "")
		default:
			return nil, fmt.Errorf("unexpected cli value type: %T", values[i])
		}
	}
	context := cli.NewContext(nil, set, nil)
	return context, nil
}

func TestGetConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()
	defer teardown(tempFile)

	app := cli.NewApp()
	app.Writer = ioutil.Discard

	tc := []struct {
		name     string
		value    string
		expected *cfg.Config
	}{
		{"", "", cfg.DefaultConfig()},
		{"config", tempFile.Name(), cfgClone},
	}

	for _, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, "")
		context := cli.NewContext(app, set, nil)

		fig, err := getConfig(context)
		if err != nil {
			t.Fatalf("failed to set fig %v", err)
		}

		if !reflect.DeepEqual(fig, c.expected) {
			t.Errorf("\ngot: %+v \nexpected: %+v", fig, c.expected)
		}
	}
}

func TestSetGlobalConfig(t *testing.T) {
	tempPath, _ := filepath.Abs("test1")
	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.GlobalConfig
	}{
		{"datadir flag",
			[]string{"datadir"},
			[]interface{}{"test1"},
			cfg.GlobalConfig{DataDir: tempPath},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			if err != nil {
				t.Fatal(err)
			}

			tCfg := &cfg.GlobalConfig{}

			setGlobalConfig(context, tCfg)

			if !reflect.DeepEqual(*tCfg, c.expected) {
				t.Errorf("\ngot: %+v \nexpected: %+v", tCfg, c.expected)
			}
		})
	}
}

func TestCreateP2PService(t *testing.T) {
	srv, _ := createP2PService(cfg.DefaultConfig())

	if srv == nil {
		t.Fatalf("failed to create p2p service")
	}
}

func TestSetP2pConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()
	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.P2pCfg
	}{
		{
			"config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()},
			cfgClone.P2p,
		},
		{
			"no bootstrap, no mdns",
			[]string{"nobootstrap", "nomdns"},
			[]interface{}{true, true},
			cfg.P2pCfg{
				BootstrapNodes: cfg.DefaultP2PBootstrap,
				Port:           cfg.DefaultP2PPort,
				NoBootstrap:    true,
				NoMdns:         true,
			},
		},
		{
			"bootstrap nodes",
			[]string{"bootnodes"},
			[]interface{}{"1234,5678"},
			cfg.P2pCfg{
				BootstrapNodes: []string{"1234", "5678"},
				Port:           cfg.DefaultP2PPort,
				NoBootstrap:    false,
				NoMdns:         false,
			},
		},
		{
			"port",
			[]string{"p2pport"},
			[]interface{}{uint(1337)},
			cfg.P2pCfg{
				BootstrapNodes: cfg.DefaultP2PBootstrap,
				Port:           1337,
				NoBootstrap:    false,
				NoMdns:         false,
			},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			if err != nil {
				t.Fatal(err)
			}

			input := cfg.DefaultConfig()
			// Must call global setup to set data dir
			setP2pConfig(context, &input.P2p)

			if !reflect.DeepEqual(input.P2p, c.expected) {
				t.Fatalf("\ngot %+v\nexpected %+v", input.P2p, c.expected)
			}
		})
	}
}

func TestSetRpcConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.RpcCfg
	}{
		{
			"config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()},
			cfgClone.Rpc,
		},
		{
			"host and port",
			[]string{"rpchost", "rpcport"},
			[]interface{}{"someHost", uint(1337)},
			cfg.RpcCfg{
				Port:    1337,
				Host:    "someHost",
				Modules: cfg.DefaultRpcModules,
			},
		},
		{
			"modules",
			[]string{"rpcmods"},
			[]interface{}{"system,state"},
			cfg.RpcCfg{
				Port:    cfg.DefaultRpcHttpPort,
				Host:    cfg.DefaultRpcHttpHost,
				Modules: []api.Module{"system", "state"},
			},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			if err != nil {
				t.Fatal(err)
			}

			input := cfg.DefaultConfig()
			setRpcConfig(context, &input.Rpc)

			if !reflect.DeepEqual(input.Rpc, c.expected) {
				t.Fatalf("\ngot %+v\nexpected %+v", input.Rpc, c.expected)
			}
		})
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
	defer teardown(tempFile)
	defer removeTestDataDir()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		flags    []string
		values   []interface{}
		expected *cfg.Config
	}{
		{"node from config (norpc)", []string{"config"}, []interface{}{tempFile.Name()}, cfgClone},
		{"default node (norpc)", []string{}, []interface{}{}, cfgClone},
		{"default node (rpc)", []string{"rpc"}, []interface{}{true}, cfgClone},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.name, func(t *testing.T) {
			context, err := createCliContext(c.name, c.flags, c.values)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = makeNode(context)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommands(t *testing.T) {
	tempFile, _ := createTempConfigFile()
	defer teardown(tempFile)
	defer removeTestDataDir()

	tc := []struct {
		description string
		flags       []string
		values      []interface{}
	}{
		{"from config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()}},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive

		app := cli.NewApp()
		app.Writer = ioutil.Discard

		context, err := createCliContext(c.description, c.flags, c.values)
		if err != nil {
			t.Fatal(err)
		}

		command := dumpConfigCommand

		err = command.Run(context)
		if err != nil {
			t.Fatalf("should have ran dumpConfig command. err: %s", err)
		}
	}
}
