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
		certOpt.CADuration,
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
	if certOpt.User == "" {
		return errors.New("a user name is required")
	}

	return createClientPair(
		certOpt.CertsDir,
		certOpt.CAKey,
		defaultNodeCert,
		certOpt.KeySize,
		certOpt.Duration,
		certOpt.Force,
		certOpt.User,
	)
}

func runList() error {
	var fileList [][4]string
	var widths [4]int

	if err := os.Chdir(certOpt.CertsDir); err != nil {
		return err
	}

	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	fmt.Printf("Scanning: %s ...\n\n", certOpt.CertsDir)

	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			ci := certInfo(path)
			dexp, mode := fmt.Sprintf("%x", ci), info.Mode().String()

			fileList = append(fileList, [4]string{
				ci.Name, path, mode, dexp,
			})

			widths[0] = max(widths[0], len(ci.Name))
			widths[1] = max(widths[1], len(path))
			widths[2] = max(widths[2], len(mode))
			widths[3] = max(widths[3], len(dexp))

			return nil
		})
	if err != nil {
		return err
	}

	fmt.Printf("%-[2]*[1]s | %-[4]*[3]s | %-[6]*[5]s | %-[8]*[7]s\n",
		"Name", widths[0],
		"File", widths[1],
		"Mode", widths[2],
		"Expires", widths[3],
	)

	fmt.Printf("%s\n", strings.Repeat("=", widths[0]+widths[1]+widths[2]+widths[3]+9))

	for i := range fileList {
		fmt.Printf("%-[2]*[1]s | %-[4]*[3]s | %-[6]*[5]s | %-[8]*[7]s\n",
			fileList[i][0], widths[0],
			fileList[i][1], widths[1],
			fileList[i][2], widths[2],
			fileList[i][3], widths[3],
		)
	}

	return nil
}
