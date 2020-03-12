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
	"path/filepath"

	log "github.com/ChainSafe/log15"
	"github.com/naoina/toml"
	"github.com/urfave/cli"
)

// exportAction is the action for the "export" subcommand
func exportAction(ctx *cli.Context) error {
	err := startLogger(ctx)
	if err != nil {
		log.Error("[cmd] Failed to start logger", "error", err)
		return err
	}

	cfg, err := createDotConfig(ctx)
	if err != nil {
		return err
	}

	comment := ""

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	export := os.Stdout

	if ctx.NArg() > 0 {
		/* #nosec */
		export, err = os.OpenFile(filepath.Clean(ctx.Args().Get(0)), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer func() {
			err = export.Close()
			if err != nil {
				log.Error("[cmd] Failed to close connection", "error", err)
			}
		}()
	}

	_, err = export.WriteString(comment)
	if err != nil {
		log.Error("[cmd] Failed to write output for export command", "error", err)
	}

	_, err = export.Write(out)
	if err != nil {
		log.Error("[cmd] Failed to write output for export command", "error", err)
	}

	return nil
}
