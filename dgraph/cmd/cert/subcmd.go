/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultCertDir      = "tls"
	defaultDuration     = time.Hour * 24 * 365
	defaultCADuration   = defaultDuration * 10
	defaultKeySize      = 2048
	defaultCAKey        = "ca.key"
	defaultCACert       = "ca.crt"
	defaultClientCAKey  = "ca-client.key"
	defaultClientCACert = "ca-client.crt"
	defaultNodeKey      = "node.key"
	defaultNodeCert     = "node.crt"
)

var certOpt struct {
	CertsDir             string
	CAKey, ClientCAKey   string
	CADuration, Duration time.Duration
	Force                bool
	KeySize              int
	Nodes                []string
}

var subcmds = []*cobra.Command{
	&cobra.Command{
		Use:   "create-ca",
		Short: "create root CA certificate and key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateCA()
		},
	},
	&cobra.Command{
		Use:   "create-client-ca",
		Short: "create client CA certificate and key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateClientCA()
		},
	},
	&cobra.Command{
		Use:   "create-node",
		Short: "create node certificates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateNode()
		},
	},
	&cobra.Command{
		Use:   "create-client",
		Short: "create client certificate and key",
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
		flag.StringVarP(&certOpt.CertsDir, "certs-dir", "d", defaultCertDir,
			"path to the directory to store TLS certs and keys")

		// list only needs certs-dir
		if subcmds[i].Use == "list" {
			continue
		}

		flag.BoolVar(&certOpt.Force, "force", false, "force overwrite any existing key and cert")
		flag.IntVarP(&certOpt.KeySize, "key-size", "b", defaultKeySize, "RSA key bit size")

		switch subcmds[i].Use {
		case "create-ca":
			flag.StringVarP(&certOpt.CAKey, "ca-key", "k", defaultCAKey, "path to the CA private key file")
			flag.DurationVar(&certOpt.CADuration, "duration", defaultCADuration, "duration of cert validity")
		case "create-client-ca":
			flag.StringVarP(&certOpt.ClientCAKey, "ca-key", "k", defaultClientCAKey, "path to the CA private key file")
			flag.DurationVar(&certOpt.CADuration, "duration", defaultCADuration, "duration of cert validity")
		case "create-node":
			flag.StringVarP(&certOpt.CAKey, "ca-key", "k", defaultCAKey, "path to the CA private key file")
			flag.DurationVar(&certOpt.Duration, "duration", defaultDuration, "duration of cert validity")
			flag.StringSliceVarP(&certOpt.Nodes, "nodes", "n", nil, "node0 ... nodeN (ipaddr | host)")
		case "create-client":
			flag.StringVarP(&certOpt.ClientCAKey, "ca-key", "k", defaultClientCAKey, "path to the CA private key file")
			flag.DurationVar(&certOpt.Duration, "duration", defaultDuration, "duration of cert validity")
		}
	}
}
