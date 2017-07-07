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
var Config ConfigOpts

func init() {
	Config.Gconf = flag.String("group_conf", "", "group configuration file")
	Config.PostingDir = flag.String("p", "p", "Directory to store posting lists.")
	Config.WalDir = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	Config.Port = flag.Int("port", 8080, "Port to run server on.")
	Config.Bindall = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	Config.Nomutations = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	Config.ExposeTrace = flag.Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	Config.Cpuprofile = flag.String("cpu", "", "write cpu profile to file")
	Config.Memprofile = flag.String("mem", "", "write memory profile to file")
	Config.BlockRate = flag.Int("block", 0, "Block profiling rate")
	Config.DumpSubgraph = flag.String("dumpsg", "",
		"Directory to save subgraph for testing, debugging")
	Config.NumPending = flag.Int("pending", 1000,
		"Number of pending queries. Useful for rate limiting.")
	// TLS configurations
	Config.TlsEnabled = flag.Bool("tls.on", false, "Use TLS connections with clients.")
	Config.TlsCert = flag.String("tls.cert", "", "Certificate file path.")
	Config.TlsKey = flag.String("tls.cert_key", "", "Certificate key file path.")
	Config.TlsKeyPass = flag.String("tls.cert_key_passphrase", "", "Certificate key passphrase.")
	Config.TlsClientAuth = flag.String("tls.client_auth", "", "Enable TLS client authentication")
	Config.TlsClientCACerts = flag.String("tls.ca_certs", "", "CA Certs file path.")
	Config.TlsSystemCACerts = flag.Bool("tls.use_system_ca", false, "Include System CA into CA Certs.")
	Config.TlsMinVersion = flag.String("tls.min_version", "TLS11", "TLS min version.")
	Config.TlsMaxVersion = flag.String("tls.max_version", "TLS12", "TLS max version.")
}
