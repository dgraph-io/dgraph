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

	"github.com/ChainSafe/gossamer/cmd/utils"
	log "github.com/inconshreveable/log15"
	"github.com/urfave/cli"
)

var (
	app       = cli.NewApp()
	nodeFlags = []cli.Flag{
		utils.DataDirFlag,
		configFileFlag,
	}
	rpcFlags = []cli.Flag{
		utils.RPCEnabledFlag,
		utils.RPCListenAddrFlag,
		utils.RPCPortFlag,
	}
)

//init initializes CLI
func init() {
	app.Action = gossamer
	app.Copyright = "Copyright 2019 Chainsafe Systems Authors"
	app.Name = "gossamer"
	app.Usage = "Official gossamer command-line interface"
	app.Author = "Chainsafe Systems 2019"
	app.Version = "0.0.1"
	app.Commands = []cli.Command{
		dumpConfigCommand,
	}
	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error("error starting app", "output", os.Stderr, "err", err)
		os.Exit(1)
	}
}

//gossamer is the main entrypoint into the gossamer system
func gossamer(ctx *cli.Context) error {
	g, err := makeNode(ctx)
	if err != nil {
		log.Error("error making node", "err", err)
	}
	log.Info("🕸️ starting p2p service", "blockchain", "gossamer")
	g.Server.Start()

	return nil
}
