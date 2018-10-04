/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Cert x.SubCommand

type options struct {
	dir, caKey, caCert, client string
	force, verify              bool
	keySize, days              int
	nodes                      []string
}

var opt options

func init() {
	Cert.Cmd = &cobra.Command{
		Use:   "cert",
		Short: "Dgraph TLS certificate management",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Cert.Conf).Stop()
			run()
		},
	}

	flag := Cert.Cmd.Flags()
	flag.StringP("dir", "d", defaultDir, "directory containing TLS certs and keys")
	flag.StringP("ca-key", "k", defaultCAKey, "path to the CA private key")
	flag.Int("keysize", defaultKeySize, "RSA key bit size for creating new keys")
	flag.Int("duration", defaultDays, "duration of cert validity in days")
	flag.StringSliceP("nodes", "n", nil, "creates cert/key pair for nodes")
	flag.StringP("client", "c", "", "create cert/key pair for a client name")
	flag.Bool("force", false, "overwrite any existing key and cert")
	flag.Bool("verify", true, "verify certs against root CA when creating")

	cmdList := &cobra.Command{
		Use:   "ls",
		Short: "lists certificates and keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listCerts()
		},
	}
	cmdList.Flags().AddFlag(Cert.Cmd.Flag("dir"))
	Cert.Cmd.AddCommand(cmdList)
}

func run() {
	opt = options{
		dir:     Cert.Conf.GetString("dir"),
		caKey:   Cert.Conf.GetString("ca-key"),
		client:  Cert.Conf.GetString("client"),
		keySize: Cert.Conf.GetInt("keysize"),
		days:    Cert.Conf.GetInt("duration"),
		nodes:   Cert.Conf.GetStringSlice("nodes"),
		force:   Cert.Conf.GetBool("force"),
		verify:  Cert.Conf.GetBool("verify"),
	}

	x.Check(createCerts(opt))
}

// listCerts handles the subcommand of "dgraph cert ls".
// This function will traverse the certs directory, "tls" by default, and
// display information about all supported files: ca.{crt,key}, node.{crt,key},
// client.{name}.{crt,key}. Any other files are flagged as unsupported.
//
// For certificates, we want to show:
//   - CommonName
//   - Serial number
//   - Verify with current CA
//   - MD5 checksum
//   - Match with key MD5
//   - Expiration date
//   - Client name or hosts (node and client certs)
//
// For keys, we want to show:
//   - File name
//   - MD5 checksum
//
// TODO: Add support to other type of keys.
func listCerts() error {
	dir := Cert.Conf.GetString("dir")
	files, err := getDirFiles(dir)
	switch {
	case err != nil:
		return err

	case len(files) == 0:
		fmt.Println("Directory is empty:", dir)
		return nil
	}

	for _, f := range files {
		if f.err != nil {
			fmt.Printf("%s: error: %s\n\n", f.fileName, f.err)
			continue
		}
		fmt.Printf("%s %s - %s\n", f.fileMode, f.fileName, f.commonName)
		if f.issuerName != "" {
			fmt.Printf("%10s: %s\n", "Issuer", f.issuerName)
		}
		if f.verifiedCA != "" {
			fmt.Printf("%10s: %s\n", "CA Verify", f.verifiedCA)
		}
		if f.serialNumber != "" {
			fmt.Printf("%10s: %s\n", "S/N", f.serialNumber)
		}
		if !f.expireDate.IsZero() {
			fmt.Printf("%10s: %x\n", "Expiration", f)
		}
		if f.hosts != nil {
			fmt.Printf("%10s: %s\n", "Hosts", strings.Join(f.hosts, ", "))
		}
		fmt.Printf("%10s: %s\n\n", "MD5 hash", f.md5sum)
	}

	return nil
}
