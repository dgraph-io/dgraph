/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
	dir, caKey, caCert, client, curve string
	force, verify                     bool
	keySize, days                     int
	nodes                             []string
}

var opt options

func init() {
	Cert.Cmd = &cobra.Command{
		Use:   "cert",
		Short: "Dgraph TLS certificate management",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer x.StartProfile(Cert.Conf).Stop()
			return run()
		},
	}

	flag := Cert.Cmd.Flags()
	flag.StringP("dir", "d", defaultDir, "directory containing TLS certs and keys")
	flag.StringP("ca-key", "k", defaultCAKey, "path to the CA private key")
	flag.IntP("keysize", "r", defaultKeySize, "RSA key bit size for creating new keys")
	flag.StringP("elliptic-curve", "e", "",
		`ECDSA curve for private key. Values are: "P224", "P256", "P384", "P521".`)
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

func run() error {
	opt = options{
		dir:     Cert.Conf.GetString("dir"),
		caKey:   Cert.Conf.GetString("ca-key"),
		client:  Cert.Conf.GetString("client"),
		keySize: Cert.Conf.GetInt("keysize"),
		days:    Cert.Conf.GetInt("duration"),
		nodes:   Cert.Conf.GetStringSlice("nodes"),
		force:   Cert.Conf.GetBool("force"),
		verify:  Cert.Conf.GetBool("verify"),
		curve:   Cert.Conf.GetString("elliptic-curve"),
	}

	return createCerts(opt)
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
			fmt.Printf("%14s: %s\n", "Issuer", f.issuerName)
		}
		if f.verifiedCA != "" {
			fmt.Printf("%14s: %s\n", "CA Verify", f.verifiedCA)
		}
		if f.serialNumber != "" {
			fmt.Printf("%14s: %s\n", "S/N", f.serialNumber)
		}
		if !f.expireDate.IsZero() {
			fmt.Printf("%14s: %x\n", "Expiration", f)
		}
		if f.hosts != nil {
			fmt.Printf("%14s: %s\n", "Hosts", strings.Join(f.hosts, ", "))
		}
		if f.algo != "" {
			fmt.Printf("%14s: %s\n", "Algorithm", f.algo)
		}
		fmt.Printf("%14s: %s\n\n", "SHA-256 Digest", f.digest)
	}

	return nil
}
