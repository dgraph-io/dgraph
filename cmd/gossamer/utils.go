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
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh/terminal"
)

// setupLogger sets up the gossamer logger
func setupLogger(ctx *cli.Context) (log.Lvl, error) {
	handler := log.StreamHandler(os.Stdout, log.TerminalFormat())
	handler = log.CallerFileHandler(handler)

	var lvl log.Lvl

	if lvlToInt, err := strconv.Atoi(ctx.String(LogFlag.Name)); err == nil {
		lvl = log.Lvl(lvlToInt)
	} else if lvl, err = log.LvlFromString(ctx.String(LogFlag.Name)); err != nil {
		return 0, err
	}

	log.Root().SetHandler(log.LvlFilterHandler(lvl, handler))

	return lvl, nil
}

// getPassword prompts user to enter password
func getPassword(msg string) []byte {
	for {
		fmt.Println(msg)
		fmt.Print("> ")
		password, err := terminal.ReadPassword(syscall.Stdin)
		if err != nil {
			fmt.Printf("invalid input: %s\n", err)
		} else {
			fmt.Printf("\n")
			return password
		}
	}
}

// confirmMessage prompts user to confirm message and returns true if "Y"
func confirmMessage(msg string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println(msg)
	fmt.Print("> ")
	for {
		text, _ := reader.ReadString('\n')
		text = strings.Replace(text, "\n", "", -1)
		return strings.Compare("Y", text) == 0
	}
}

// newTestConfig returns a new test configuration using the provided basepath
func newTestConfig(t *testing.T) *dot.Config {
	dir := utils.NewTestDir(t)

	// TODO: use default config instead of gssmr config for test config #776

	cfg := &dot.Config{
		Global: dot.GlobalConfig{
			Name:     dot.GssmrConfig().Global.Name,
			ID:       dot.GssmrConfig().Global.ID,
			BasePath: dir,
			LogLvl:   log.LvlInfo,
		},
		Log: dot.LogConfig{
			CoreLvl:           log.LvlInfo,
			SyncLvl:           log.LvlInfo,
			NetworkLvl:        log.LvlInfo,
			RPCLvl:            log.LvlInfo,
			StateLvl:          log.LvlInfo,
			RuntimeLvl:        log.LvlInfo,
			BlockProducerLvl:  log.LvlInfo,
			FinalityGadgetLvl: log.LvlInfo,
		},
		Init:    dot.GssmrConfig().Init,
		Account: dot.GssmrConfig().Account,
		Core:    dot.GssmrConfig().Core,
		Network: dot.GssmrConfig().Network,
		RPC:     dot.GssmrConfig().RPC,
		System:  dot.GssmrConfig().System,
	}

	cfg.Init.TestFirstEpoch = true
	return cfg
}

// newTestConfigWithFile returns a new test configuration and a temporary configuration file
func newTestConfigWithFile(t *testing.T) (*dot.Config, *os.File) {
	cfg := newTestConfig(t)

	file, err := ioutil.TempFile(cfg.Global.BasePath, "config-")
	require.NoError(t, err)

	tomlCfg := dotConfigToToml(cfg)

	cfgFile := exportConfig(tomlCfg, file.Name())
	return cfg, cfgFile
}
