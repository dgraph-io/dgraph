/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cm

import (
	"github.com/spf13/cobra"
)

const (
	defaultDir      = "tls"
	defaultDays     = 1826
	defaultCACert   = "ca.crt"
	defaultCAKey    = "ca.key"
	defaultKeySize  = 2048
	defaultNodeCert = "node.crt"
	defaultNodeKey  = "node.key"
)

var opt struct {
	Dir, CAKey, User string
	Force            bool
	KeySize, Days    int
	Nodes            []string
}

var subcmds = []*cobra.Command{
	&cobra.Command{
		Use:   "make-ca",
		Short: "create root CA certificate and key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateCA()
		},
	},
	&cobra.Command{
		Use:   "make-node",
		Short: "create node certificates and keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateNode()
		},
	},
	&cobra.Command{
		Use:   "make-client",
		Short: "create client a certificate and key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateClient()
		},
	},
	&cobra.Command{
		Use:   "list",
		Short: "list certificates and keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	},
}

func flagInit() {
	for i := range subcmds {
		flag := subcmds[i].Flags()
		flag.StringVarP(&opt.Dir, "dir", "d", defaultDir,
			"directory where to store TLS certs and keys")

		// list only needs the dir
		if subcmds[i].Use == "list" {
			continue
		}

		flag.StringVarP(&opt.CAKey, "ca-key", "k", defaultCAKey, "path to the CA private key")
		flag.BoolVar(&opt.Force, "force", false, "overwrite any existing key and cert")
		flag.IntVar(&opt.KeySize, "key-size", defaultKeySize, "RSA key bit size")
		flag.IntVar(&opt.Days, "duration", defaultDays, "duration of cert validity in days")

		switch subcmds[i].Use {
		case "make-node":
			flag.StringSliceVarP(&opt.Nodes, "nodes", "n", nil, "node0 ... nodeN (ipaddr | host)")
		case "make-client":
			flag.StringVarP(&opt.User, "user", "u", "", "user name")
		}
	}
}
