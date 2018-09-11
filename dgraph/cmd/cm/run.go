/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var CM x.SubCommand

type options struct {
	dir, caKey, caCert, user string
	force                    int
	keySize, days            int
	nodes                    []string
	verify                   bool
}

var opt options

func init() {
	CM.Cmd = &cobra.Command{
		Use:   "cm",
		Short: "Dgraph certificate management",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(CM.Conf).Stop()
			run()
		},
	}

	CM.EnvPrefix = "DGRAPH_CERT"

	CM.Cmd.AddCommand()

	flag := CM.Cmd.Flags()
	flag.StringP("dir", "d", defaultDir, "directory containing TLS certs and keys")
	flag.StringP("ca-key", "k", defaultCAKey, "path to the CA private key")
	flag.Int("key-size", defaultKeySize, "RSA key bit size")
	flag.Int("duration", defaultDays, "duration of cert validity in days")
	flag.StringSliceP("nodes", "n", nil, "creates cert/key pair for nodes: node0 ... nodeN (ipaddr | host)")
	flag.StringP("user", "u", "", "create cert/key pair for a user name")
	flag.Bool("force", false, "overwrite any existing key and cert")
	flag.Bool("force-ca", false, "overwrite any CA key and cert (warning: invalidates existing signed certs)")
	flag.Bool("force-node", false, "overwrite any node key and cert")
	flag.Bool("force-client", false, "overwrite any client key and cert")
	flag.Bool("verify", true, "verify certs against root CA")

	cmdList := &cobra.Command{
		Use:   "list",
		Short: "list certificates and keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}
	cmdList.Flags().AddFlag(CM.Cmd.Flag("dir"))
	CM.Cmd.AddCommand(cmdList)
}

func run() {
	opt = options{
		dir:     CM.Conf.GetString("dir"),
		caKey:   CM.Conf.GetString("ca-key"),
		user:    CM.Conf.GetString("user"),
		keySize: CM.Conf.GetInt("key-size"),
		days:    CM.Conf.GetInt("duration"),
		nodes:   CM.Conf.GetStringSlice("nodes"),
		verify:  CM.Conf.GetBool("verify"),
	}

	if CM.Conf.GetBool("force") {
		opt.force = forceCA | forceNode | forceClient
	} else {
		if CM.Conf.GetBool("force-ca") {
			opt.force |= forceCA
		}
		if CM.Conf.GetBool("force-client") {
			opt.force |= forceClient
		}
		if CM.Conf.GetBool("force-node") {
			opt.force |= forceNode
		}
	}

	if err := createCerts(opt); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func runList() error {
	var (
		fileList [][4]string
		widths   [4]int
		dir      string
	)

	dir = CM.Conf.GetString("dir")

	if err := os.Chdir(dir); err != nil {
		return err
	}

	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	fmt.Printf("Scanning: %s ...\n\n", dir)

	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			ci := certInfo(path)
			if ci == nil {
				return nil
			}

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

	if len(fileList) == 0 {
		fmt.Println("Directory is empty:", dir)
		return nil
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
