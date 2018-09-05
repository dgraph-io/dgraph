/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Cert x.SubCommand

func init() {
	flagInit()

	Cert.Cmd = &cobra.Command{
		Use:   "cert",
		Short: "Dgraph TLS cert management",
	}
	Cert.Cmd.AddCommand(subcmds...)
	Cert.EnvPrefix = "DGRAPH_CERT"
}

func runCreateCA() error {
	return createCAPair(
		certOpt.CertsDir,
		certOpt.CAKey,
		defaultCACert,
		certOpt.KeySize,
		certOpt.Duration,
		certOpt.Force,
	)
}

func runCreateClientCA() error {
	return createCAPair(
		certOpt.CertsDir,
		certOpt.ClientCAKey,
		defaultClientCACert,
		certOpt.KeySize,
		certOpt.Duration,
		certOpt.Force,
	)
}

func runCreateNode() error {
	if certOpt.Nodes == nil || len(certOpt.Nodes) == 0 {
		return errors.New("required at least one node (ip address or host)")
	}

	return createNodePair(
		certOpt.CertsDir,
		certOpt.CAKey,
		defaultNodeCert,
		certOpt.KeySize,
		certOpt.Duration,
		certOpt.Force,
		certOpt.Nodes,
	)
}

func runCreateClient() error {
	// TBD
	return nil
}

// TODO: improve this
func runList() error {
	if err := os.Chdir(certOpt.CertsDir); err != nil {
		return err
	}

	fmt.Printf("Scanning: %s ...\n\n", certOpt.CertsDir)
	fmt.Printf("%-[4]*[1]s | %-[4]*[2]s | %[3]s\n", "Type", "File", "Expires", 20)
	fmt.Printf("%s\n", strings.Repeat("=", 68))

	return filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			fmt.Printf("%[2]*[1]t | %[2]*[1]f | %[1]x\n", certInfo(path), 20)

			return nil
		})
}
