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
	"fmt"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/urfave/cli"
)

// exportAction is the action for the "export" subcommand
func exportAction(ctx *cli.Context) error {
	// use --config value as export destination
	config := ctx.GlobalString(ConfigFlag.Name)

	// check if --config value is set
	if config == "" {
		return fmt.Errorf("export destination undefined: --config value required")
	}

	// check if configuration file already exists at export destination
	if utils.PathExists(config) {
		logger.Warn(
			"toml configuration file already exists",
			"config", config,
		)

		// use --force value to force overwrite the toml configuration file
		force := ctx.Bool(ForceFlag.Name)

		// prompt user to confirm overwriting existing toml configuration file
		if force || confirmMessage("Are you sure you want to overwrite the file? [Y/n]") {
			logger.Warn(
				"overwriting toml configuration file",
				"config", config,
			)
		} else {
			logger.Warn(
				"exiting without exporting toml configuration file",
				"config", config,
			)
			return nil // exit if reinitialization is not confirmed
		}
	}

	cfg, err := createExportConfig(ctx)
	if err != nil {
		return err
	}

	file := dot.ExportConfig(cfg, config)
	// export config will exit and log error on error

	logger.Info("exported toml configuration file", "path", file.Name())

	return nil
}
