/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package server

import (
	"flag"
)

type ConfigOpts struct {
	Gconf        *string
	PostingDir   *string
	WalDir       *string
	Port         *int
	Bindall      *bool
	Nomutations  *bool
	ExposeTrace  *bool
	Cpuprofile   *string
	Memprofile   *string
	BlockRate    *int
	DumpSubgraph *string
	NumPending   *int
	// TLS configurations
	TlsEnabled       *bool
	TlsCert          *string
	TlsKey           *string
	TlsKeyPass       *string
	TlsClientAuth    *string
	TlsClientCACerts *string
	TlsSystemCACerts *bool
	TlsMinVersion    *string
	TlsMaxVersion    *string
}

// TODO(tzdybal) - remove global
var Config = NewConfig()

func NewConfig() (config ConfigOpts) {
	config.Gconf = flag.String("group_conf", "", "group configuration file")
	config.PostingDir = flag.String("p", "p", "Directory to store posting lists.")
	config.WalDir = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	config.Port = flag.Int("port", 8080, "Port to run server on.")
	config.Bindall = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	config.Nomutations = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	config.ExposeTrace = flag.Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	config.Cpuprofile = flag.String("cpu", "", "write cpu profile to file")
	config.Memprofile = flag.String("mem", "", "write memory profile to file")
	config.BlockRate = flag.Int("block", 0, "Block profiling rate")
	config.DumpSubgraph = flag.String("dumpsg", "",
		"Directory to save subgraph for testing, debugging")
	config.NumPending = flag.Int("pending", 1000,
		"Number of pending queries. Useful for rate limiting.")
	// TLS configurations
	config.TlsEnabled = flag.Bool("tls.on", false, "Use TLS connections with clients.")
	config.TlsCert = flag.String("tls.cert", "", "Certificate file path.")
	config.TlsKey = flag.String("tls.cert_key", "", "Certificate key file path.")
	config.TlsKeyPass = flag.String("tls.cert_key_passphrase", "", "Certificate key passphrase.")
	config.TlsClientAuth = flag.String("tls.client_auth", "", "Enable TLS client authentication")
	config.TlsClientCACerts = flag.String("tls.ca_certs", "", "CA Certs file path.")
	config.TlsSystemCACerts = flag.Bool("tls.use_system_ca", false, "Include System CA into CA Certs.")
	config.TlsMinVersion = flag.String("tls.min_version", "TLS11", "TLS min version.")
	config.TlsMaxVersion = flag.String("tls.max_version", "TLS12", "TLS max version.")
	return config
}
