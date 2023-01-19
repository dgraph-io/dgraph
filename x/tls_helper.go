/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dgraph-io/ristretto/z"
)

// TLSHelperConfig define params used to create a tls.Config
type TLSHelperConfig struct {
	CertRequired     bool
	Cert             string
	Key              string
	ServerName       string
	RootCACert       string
	ClientAuth       string
	UseSystemCACerts bool
}

const (
	TLSDefaults = `use-system-ca=true; client-auth-type=VERIFYIFGIVEN; internal-port=false; ` +
		`ca-cert=; server-name=; server-cert=; server-key=; client-cert=; client-key=;`

	TLSServerDefaults = `use-system-ca=true; client-auth-type=VERIFYIFGIVEN; internal-port=false; ` +
		`server-cert=; server-key=; ca-cert=; client-cert=; client-key=;`

	TLSClientDefaults = `use-system-ca=true; internal-port=false; server-name=; ca-cert=; ` +
		`client-cert=; client-key=;`
)

// RegisterServerTLSFlags registers the required flags to set up a TLS server.
func RegisterServerTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls", "use-system-ca=true; client-auth-type=VERIFYIFGIVEN; internal-port=false;",
		z.NewSuperFlagHelp(TLSServerDefaults).
			Head("TLS Server options").
			Flag("internal-port",
				"(Optional) Enable inter-node TLS encryption between cluster nodes.").
			Flag("server-cert",
				"The server Cert file which is needed to initiate the server in the cluster.").
			Flag("server-key",
				"The server Key file which is needed to initiate the server in the cluster.").
			Flag("ca-cert",
				"The CA cert file used to verify server certificates. Required for enabling TLS.").
			Flag("use-system-ca",
				"Includes System CA into CA Certs.").
			Flag("client-auth-type",
				"The TLS client authentication method.").
			Flag("client-cert",
				"(Optional) The client Cert file which is needed to connect as a client with the other "+
					"nodes in the cluster.").
			Flag("client-key",
				"(Optional) The private client Key file which is needed to connect as a client with the "+
					"other nodes in the cluster.").
			String())
}

// RegisterClientTLSFlags registers the required flags to set up a TLS client.
func RegisterClientTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls", "use-system-ca=true; internal-port=false;",
		z.NewSuperFlagHelp(TLSClientDefaults).
			Head("TLS Client options").
			Flag("internal-port",
				"(Optional) Enable inter-node TLS encryption between cluster nodes.").
			Flag("server-name",
				"Used to verify the server hostname.").
			Flag("ca-cert",
				"The CA cert file used to verify server certificates. Required for enabling TLS.").
			Flag("use-system-ca",
				"Includes System CA into CA Certs.").
			Flag("client-cert",
				"(Optional) The Cert file provided by the client to the server.").
			Flag("client-key",
				"(Optional) The private Key file provided by the clients to the server.").
			String())
}

// LoadClientTLSConfigForInternalPort loads tls config for connecting to internal ports of cluster
func LoadClientTLSConfigForInternalPort(v *viper.Viper) (*tls.Config, error) {
	tlsFlag := z.NewSuperFlag(v.GetString("tls")).MergeAndCheckDefault(TLSDefaults)

	if !tlsFlag.GetBool("internal-port") {
		return nil, nil
	}
	if tlsFlag.GetPath("client-cert") == "" || tlsFlag.GetPath("client-key") == "" {
		return nil, errors.Errorf(`Inter-node TLS is enabled but client certs are not provided. ` +
			`Inter-node TLS is always client authenticated. Please provide --tls ` +
			`"client-cert=...; client-key=...;"`)
	}

	conf := &TLSHelperConfig{}
	conf.UseSystemCACerts = tlsFlag.GetBool("use-system-ca")
	conf.RootCACert = tlsFlag.GetPath("ca-cert")
	conf.CertRequired = true
	conf.Cert = tlsFlag.GetPath("client-cert")
	conf.Key = tlsFlag.GetPath("client-key")
	return GenerateClientTLSConfig(conf)
}

// LoadServerTLSConfigForInternalPort loads the TLS config for the internal ports of the cluster
func LoadServerTLSConfigForInternalPort(v *viper.Viper) (*tls.Config, error) {
	tlsFlag := z.NewSuperFlag(v.GetString("tls")).MergeAndCheckDefault(TLSDefaults)

	if !tlsFlag.GetBool("internal-port") {
		return nil, nil
	}
	if tlsFlag.GetPath("server-cert") == "" || tlsFlag.GetPath("server-key") == "" {
		return nil, errors.Errorf(`Inter-node TLS is enabled but server node certs are not provided. ` +
			`Please provide --tls "server-cert=...; server-key=...;"`)
	}
	conf := TLSHelperConfig{}
	conf.UseSystemCACerts = tlsFlag.GetBool("use-system-ca")
	conf.RootCACert = tlsFlag.GetPath("ca-cert")
	conf.CertRequired = true
	conf.Cert = tlsFlag.GetPath("server-cert")
	conf.Key = tlsFlag.GetPath("server-key")
	conf.ClientAuth = "REQUIREANDVERIFY"
	return GenerateServerTLSConfig(&conf)
}

// LoadServerTLSConfig loads the TLS config into the server with the given parameters.
func LoadServerTLSConfig(v *viper.Viper) (*tls.Config, error) {
	tlsFlag := z.NewSuperFlag(v.GetString("tls")).MergeAndCheckDefault(TLSDefaults)

	if tlsFlag.GetPath("server-cert") == "" && tlsFlag.GetPath("server-key") == "" {
		return nil, nil
	}

	conf := TLSHelperConfig{}
	conf.RootCACert = tlsFlag.GetPath("ca-cert")
	conf.CertRequired = true
	conf.Cert = tlsFlag.GetPath("server-cert")
	conf.Key = tlsFlag.GetPath("server-key")
	conf.ClientAuth = tlsFlag.GetString("client-auth-type")
	conf.UseSystemCACerts = tlsFlag.GetBool("use-system-ca")
	return GenerateServerTLSConfig(&conf)
}

// SlashTLSConfig returns the TLS config appropriate for SlashGraphQL
// This assumes that endpoint is not empty, and in the format "domain.grpc.cloud.dg.io:443"
func SlashTLSConfig(endpoint string) (*tls.Config, error) {
	pool, err := generateCertPool("", true)
	if err != nil {
		return nil, err
	}
	hostWithoutPort := strings.Split(endpoint, ":")[0]
	return &tls.Config{
		RootCAs:    pool,
		ServerName: hostWithoutPort,
		MinVersion: tls.VersionTLS12,
	}, nil
}

// LoadClientTLSConfig loads the TLS config into the client with the given parameters.
func LoadClientTLSConfig(v *viper.Viper) (*tls.Config, error) {
	if v.GetString("slash_grpc_endpoint") != "" {
		return SlashTLSConfig(v.GetString("slash_grpc_endpoint"))
	}

	tlsFlag := z.NewSuperFlag(v.GetString("tls")).MergeAndCheckDefault(TLSDefaults)

	// When the --tls ca-cert="..."; option is specified, the connection will be set up using TLS
	// instead of plaintext. However the client cert files are optional, depending on whether the
	// server requires a client certificate.
	caCert := tlsFlag.GetPath("ca-cert")
	if caCert != "" {
		tlsCfg := tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		// 1. set up the root CA
		pool, err := generateCertPool(caCert, tlsFlag.GetBool("use-system-ca"))
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool

		// 2. set up the server name for verification
		tlsCfg.ServerName = tlsFlag.GetString("server-name")

		// 3. optionally load the client cert files
		certFile := tlsFlag.GetPath("client-cert")
		keyFile := tlsFlag.GetPath("client-key")
		if certFile != "" && keyFile != "" {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return nil, err
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}

		return &tlsCfg, nil
	} else
	// Attempt to determine if user specified *any* TLS option. Unfortunately and contrary to
	// Viper's own documentation, there's no way to tell whether an option value came from a
	// command-line option or a built-it default.
	if tlsFlag.GetString("server-name") != "" ||
		tlsFlag.GetPath("client-cert") != "" ||
		tlsFlag.GetPath("client-key") != "" {
		return nil, errors.Errorf(`--tls "ca-cert=...;" is required for enabling TLS`)
	}
	return nil, nil
}

func generateCertPool(certPath string, useSystemCA bool) (*x509.CertPool, error) {
	var pool *x509.CertPool
	if useSystemCA {
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
	} else {
		pool = x509.NewCertPool()
	}

	if len(certPath) > 0 {
		caFile, err := ioutil.ReadFile(certPath)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(caFile) {
			return nil, errors.Errorf("error reading CA file %q", certPath)
		}
	}

	return pool, nil
}

func setupClientAuth(authType string) (tls.ClientAuthType, error) {
	auth := map[string]tls.ClientAuthType{
		"REQUEST":          tls.RequestClientCert,
		"REQUIREANY":       tls.RequireAnyClientCert,
		"VERIFYIFGIVEN":    tls.VerifyClientCertIfGiven,
		"REQUIREANDVERIFY": tls.RequireAndVerifyClientCert,
	}

	if len(authType) > 0 {
		if v, has := auth[strings.ToUpper(authType)]; has {
			return v, nil
		}
		return tls.NoClientCert, errors.Errorf("Invalid client auth. Valid values " +
			"[REQUEST, REQUIREANY, VERIFYIFGIVEN, REQUIREANDVERIFY]")
	}

	return tls.NoClientCert, nil
}

// TLSBaseConfig returns a *tls.Config with the base set of security
// requirements (minimum TLS v1.2 and set of cipher suites)
func TLSBaseConfig() *tls.Config {
	tlsCfg := new(tls.Config)
	tlsCfg.MinVersion = tls.VersionTLS12
	tlsCfg.CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	}
	return tlsCfg
}

// GenerateServerTLSConfig creates and returns a new *tls.Config with the
// configuration provided.
func GenerateServerTLSConfig(config *TLSHelperConfig) (tlsCfg *tls.Config, err error) {
	if config.CertRequired {
		tlsCfg = TLSBaseConfig()
		cert, err := tls.LoadX509KeyPair(config.Cert, config.Key)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}

		pool, err := generateCertPool(config.RootCACert, config.UseSystemCACerts)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = pool

		auth, err := setupClientAuth(config.ClientAuth)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientAuth = auth

		return tlsCfg, nil
	}
	return nil, nil
}

// GenerateClientTLSConfig creates and returns a new client side *tls.Config with the
// configuration provided.
func GenerateClientTLSConfig(config *TLSHelperConfig) (tlsCfg *tls.Config, err error) {
	if config.CertRequired {
		tlsCfg := tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		// 1. set up the root CA
		pool, err := generateCertPool(config.RootCACert, config.UseSystemCACerts)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool

		// 2. set up the server name for verification
		tlsCfg.ServerName = config.ServerName

		// 3. optionally load the client cert files
		certFile := config.Cert
		keyFile := config.Key
		if certFile != "" && keyFile != "" {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return nil, err
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}

		return &tlsCfg, nil
	}

	return nil, nil
}
