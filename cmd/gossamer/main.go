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
	"os"
	"strconv"

	"github.com/ChainSafe/gossamer/cmd/utils"
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

var (
	app       = cli.NewApp()
	nodeFlags = []cli.Flag{
		utils.DataDirFlag,
		configFileFlag,
	}
	p2pFlags = []cli.Flag{
		utils.BootnodesFlag,
		utils.NoBootstrapFlag,
	}
	rpcFlags = []cli.Flag{
		utils.RpcEnabledFlag,
		utils.RpcListenAddrFlag,
		utils.RpcPortFlag,
		utils.RpcHostFlag,
		utils.RpcModuleFlag,
	}
	genesisFlags = []cli.Flag{
		utils.GenesisFlag,
	}
	cliFlags = []cli.Flag{
		utils.VerbosityFlag,
	}
)

// init initializes CLI
func init() {
	app.Action = gossamer
	app.Copyright = "Copyright 2019 ChainSafe Systems Authors"
	app.Name = "gossamer"
	app.Usage = "Official gossamer command-line interface"
	app.Author = "ChainSafe Systems 2019"
	app.Version = "0.0.1"
	app.Commands = []cli.Command{
		dumpConfigCommand,
	}
	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, p2pFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, genesisFlags...)
	app.Flags = append(app.Flags, cliFlags...)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error("error starting app", "err", err)
		os.Exit(1)
	}
}

func startLogger(ctx *cli.Context) error {
	logger := log.Root()
	handler := logger.GetHandler()
	var lvl log.Lvl

	if lvlToInt, err := strconv.Atoi(ctx.String(utils.VerbosityFlag.Name)); err == nil {
		lvl = log.Lvl(lvlToInt)
	} else if lvl, err = log.LvlFromString(ctx.String(utils.VerbosityFlag.Name)); err != nil {
		return err
	}
	log.Root().SetHandler(log.LvlFilterHandler(lvl, handler))

	return nil
}

// gossamer is the main entrypoint into the gossamer system
func gossamer(ctx *cli.Context) error {
	genesisState, err := loadGenesis(ctx)
	if err != nil {
		log.Error("error loading genesis state", "error", err)
	}

	err = startLogger(ctx)
	if err != nil {
		return err
	}

	node, _, err := makeNode(ctx, genesisState)
	if err != nil {
		// TODO: Need to manage error propagation and exit smoothly
		return err
	}

	log.Info("🕸️Starting node...", "name", genesisState.Name, "ID", genesisState.Id)
	node.Start()

	return nil
}
