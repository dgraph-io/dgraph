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
	"os"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

// app is the cli application
var app = cli.NewApp()
var logger = log.New("pkg", "cmd")

var (
	// exportCommand defines the "export" subcommand (ie, `gossamer export`)
	exportCommand = cli.Command{
		Action:    FixFlagOrder(exportAction),
		Name:      "export",
		Usage:     "Export configuration values to TOML configuration file",
		ArgsUsage: "",
		Flags:     ExportFlags,
		Category:  "EXPORT",
		Description: "The export command exports configuration values from the command flags to a TOML configuration file.\n" +
			"\tUsage: gossamer export --config chain/test/config.toml --basepath ~/.gossamer/test",
	}
	// initCommand defines the "init" subcommand (ie, `gossamer init`)
	initCommand = cli.Command{
		Action:    FixFlagOrder(initAction),
		Name:      "init",
		Usage:     "Initialize node databases and load genesis data to state",
		ArgsUsage: "",
		Flags:     InitFlags,
		Category:  "INIT",
		Description: "The init command initializes the node databases and loads the genesis data from the raw genesis configuration file to state.\n" +
			"\tUsage: gossamer init --genesis-raw genesis.json",
	}
	// accountCommand defines the "account" subcommand (ie, `gossamer account`)
	accountCommand = cli.Command{
		Action:   FixFlagOrder(accountAction),
		Name:     "account",
		Usage:    "Create and manage node keystore accounts",
		Flags:    AccountFlags,
		Category: "ACCOUNT",
		Description: "The account command is used to manage the gossamer keystore.\n" +
			"\tTo generate a new sr25519 account: gossamer account --generate\n" +
			"\tTo generate a new ed25519 account: gossamer account --generate --ed25519\n" +
			"\tTo generate a new secp256k1 account: gossamer account --generate --secp256k1\n" +
			"\tTo import a keystore file: gossamer account --import=path/to/file\n" +
			"\tTo list keys: gossamer account --list",
	}
	// initCommand defines the "init" subcommand (ie, `gossamer init`)
	buildSpecCommand = cli.Command{
		Action:    FixFlagOrder(buildSpecAction),
		Name:      "build-spec",
		Usage:     "Generates genesis JSON data, and can convert to raw genesis data",
		ArgsUsage: "",
		Flags:     BuildSpecFlags,
		Category:  "BUILD-SPEC",
		Description: "The build-spec command outputs current genesis JSON data.\n" +
			"\tUsage: gossamer build-spec\n" +
			"\tTo generate raw genesis file: gossamer build-spec --raw",
	}
)

// init initializes the cli application
func init() {
	app.Action = gossamerAction
	app.Copyright = "Copyright 2019 ChainSafe Systems Authors"
	app.Name = "gossamer"
	app.Usage = "Official gossamer command-line interface"
	app.Author = "ChainSafe Systems 2019"
	app.Version = "0.0.1"
	app.Commands = []cli.Command{
		exportCommand,
		initCommand,
		accountCommand,
		buildSpecCommand,
	}
	app.Flags = RootFlags
}

// main runs the cli application
func main() {
	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// gossamerAction is the root action for the gossamer command, creates a node
// configuration, loads the keystore, initializes the node if not initialized,
// then creates and starts the node and node services
func gossamerAction(ctx *cli.Context) error {
	// check for unknown command arguments
	if arguments := ctx.Args(); len(arguments) > 0 {
		return fmt.Errorf("failed to read command argument: %q", arguments[0])
	}

	// begin profiling, if set
	stopFunc, err := beginProfile(ctx)
	if err != nil {
		return err
	}

	// setup gossamer logger
	lvl, err := setupLogger(ctx)
	if err != nil {
		logger.Error("failed to setup logger", "error", err)
		return err
	}

	// create new dot configuration (the dot configuration is created within the
	// cli application from the flag values provided)
	cfg, err := createDotConfig(ctx)
	if err != nil {
		logger.Error("failed to create node configuration", "error", err)
		return err
	}

	cfg.Global.LogLevel = lvl.String()

	// expand data directory and update node configuration (performed separately
	// from createDotConfig because dot config should not include expanded path)
	cfg.Global.BasePath = utils.ExpandDir(cfg.Global.BasePath)

	// check if node has not been initialized (expected true - add warning log)
	if !dot.NodeInitialized(cfg.Global.BasePath, true) {

		// initialize node (initialize state database and load genesis data)
		err = dot.InitNode(cfg)
		if err != nil {
			logger.Error("failed to initialize node", "error", err)
			return err
		}
	}

	// ensure configuration matches genesis data stored during node initialization
	// but do not overwrite configuration if the corresponding flag value is set
	err = updateDotConfigFromGenesisData(ctx, cfg)
	if err != nil {
		logger.Error("failed to update config from genesis data", "error", err)
		return err
	}

	ks := keystore.NewGlobalKeystore()
	err = keystore.LoadKeystore(cfg.Account.Key, ks.Acco)
	if err != nil {
		logger.Error("failed to load account keystore", "error", err)
		return err
	}

	err = keystore.LoadKeystore(cfg.Account.Key, ks.Babe)
	if err != nil {
		logger.Error("failed to load BABE keystore", "error", err)
		return err
	}

	err = keystore.LoadKeystore(cfg.Account.Key, ks.Gran)
	if err != nil {
		logger.Error("failed to load grandpa keystore", "error", err)
		return err
	}

	err = unlockKeystore(ks.Acco, cfg.Global.BasePath, cfg.Account.Unlock, ctx.String(PasswordFlag.Name))
	if err != nil {
		logger.Error("failed to unlock keystore", "error", err)
		return err
	}

	err = unlockKeystore(ks.Babe, cfg.Global.BasePath, cfg.Account.Unlock, ctx.String(PasswordFlag.Name))
	if err != nil {
		logger.Error("failed to unlock keystore", "error", err)
		return err
	}

	err = unlockKeystore(ks.Gran, cfg.Global.BasePath, cfg.Account.Unlock, ctx.String(PasswordFlag.Name))
	if err != nil {
		logger.Error("failed to unlock keystore", "error", err)
		return err
	}

	node, err := dot.NewNode(cfg, ks, stopFunc)
	if err != nil {
		logger.Error("failed to create node services", "error", err)
		return err
	}

	logger.Info("starting node...", "name", node.Name)

	// start node
	err = node.Start()
	if err != nil {
		return err
	}

	return nil
}

// initAction is the action for the "init" subcommand, initializes the trie and
// state databases and loads initial state from the configured genesis file
func initAction(ctx *cli.Context) error {
	lvl, err := setupLogger(ctx)
	if err != nil {
		logger.Error("failed to setup logger", "error", err)
		return err
	}

	cfg, err := createInitConfig(ctx)
	if err != nil {
		logger.Error("failed to create node configuration", "error", err)
		return err
	}

	cfg.Global.LogLevel = lvl.String()

	// expand data directory and update node configuration (performed separately
	// from createDotConfig because dot config should not include expanded path)
	cfg.Global.BasePath = utils.ExpandDir(cfg.Global.BasePath)

	// check if node has been initialized (expected false - no warning log)
	if dot.NodeInitialized(cfg.Global.BasePath, false) {

		// use --force value to force initialize the node
		force := ctx.Bool(ForceFlag.Name)

		// prompt user to confirm reinitialization
		if force || confirmMessage("Are you sure you want to reinitialize the node? [Y/n]") {
			logger.Info(
				"reinitializing node...",
				"basepath", cfg.Global.BasePath,
			)
		} else {
			logger.Warn(
				"exiting without reinitializing the node",
				"basepath", cfg.Global.BasePath,
			)
			return nil // exit if reinitialization is not confirmed
		}
	}

	// initialize node (initialize state database and load genesis data)
	err = dot.InitNode(cfg)
	if err != nil {
		logger.Error("failed to initialize node", "error", err)
		return err
	}

	return nil
}

func buildSpecAction(ctx *cli.Context) error {
	// set logger to critical, so output only contains genesis data
	err := ctx.Set("log", "crit")
	if err != nil {
		return err
	}
	_, err = setupLogger(ctx)
	if err != nil {
		return err
	}

	var bs *dot.BuildSpec
	if genesis := ctx.String(GenesisFlag.Name); genesis != "" {
		bspec, e := dot.BuildFromGenesis(genesis)
		if e != nil {
			return e
		}
		bs = bspec
	} else {
		cfg, e := createBuildSpecConfig(ctx)
		if e != nil {
			return e
		}
		// expand data directory and update node configuration (performed separately
		// from createDotConfig because dot config should not include expanded path)
		cfg.Global.BasePath = utils.ExpandDir(cfg.Global.BasePath)

		bspec, e := dot.BuildFromDB(cfg.Global.BasePath)
		if e != nil {
			return fmt.Errorf("error building spec from database, init must be run before build-spec or run build-spec with --genesis flag Error %s", e)
		}
		bs = bspec
	}

	if bs == nil {
		return fmt.Errorf("error building genesis")
	}

	res := []byte{} //nolint
	if ctx.Bool(RawFlag.Name) {
		res, err = bs.ToJSONRaw()
	} else {
		res, err = bs.ToJSON()
	}
	if err != nil {
		return err
	}
	// TODO implement --output flag so that user can specify redirecting output a file.
	//   then this can be removed (See issue #1029)
	fmt.Printf("%s", res)

	return nil
}
