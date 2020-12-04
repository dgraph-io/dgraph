/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

// RegisterServerTLSFlags registers the required flags to set up a TLS server.
func RegisterServerTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls-cacert", "",
		"The CA Cert file used to initiate server certificates. Required for enabling TLS.")
	flag.Bool("tls-use-system-ca", true, "Include System CA into CA Certs.")
	flag.String("tls-client-auth", "VERIFYIFGIVEN", "Enable TLS client authentication")
	flag.String("tls-node-cert", "", "The node Cert file which is needed to "+
		"initiate server in the cluster.")
	flag.String("tls-node-key", "", "The node key file "+
		"which is needed to initiate server in the cluster.")
	flag.Bool("tls-internal-port-enabled", false,
		"(optional) enable inter node TLS encryption between cluster nodes.")
	flag.String("tls-cert", "", "(optional) The client Cert file which is needed to "+
		"connect as a client with the other nodes in the cluster.")
	flag.String("tls-key", "", "(optional) The private client key file "+
		"which is needed to connect as a client with the other nodes in the cluster.")
}

// RegisterClientTLSFlags registers the required flags to set up a TLS client.
func RegisterClientTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls-cacert", "",
		"The CA Cert file used to verify server certificates. Required for enabling TLS.")
	flag.Bool("tls-use-system-ca", true, "Include System CA into CA Certs.")
	flag.String("tls-server-name", "", "Used to verify the server hostname.")
	flag.String("tls-cert", "", "(optional) The Cert file provided by the client to the server.")
	flag.String("tls-key", "", "(optional) The private key file "+
		"provided by the client to the server.")
	flag.Bool("tls-internal-port-enabled", false, "enable inter node TLS encryption between cluster nodes.")
}

// LoadClientTLSConfigForInternalPort loads tls config for connecting to internal ports of cluster
func LoadClientTLSConfigForInternalPort(v *viper.Viper) (*tls.Config, error) {
	if !v.GetBool("tls-internal-port-enabled") {
		return nil, nil
	}
	if v.GetString("tls-cert") == "" || v.GetString("tls-key") == "" {
		return nil, errors.Errorf("inter node tls is enabled but client certs are not provided. " +
			"Intern Node TLS is always client authenticated. Please provide --tls_cert and --tls_key")
	}

	conf := &TLSHelperConfig{}
	conf.UseSystemCACerts = v.GetBool("tls-use-system-ca")
	conf.RootCACert = v.GetString("tls-cacert")
	conf.CertRequired = true
	conf.Cert = v.GetString("tls-cert")
	conf.Key = v.GetString("tls-key")
	return GenerateClientTLSConfig(conf)
}

// LoadServerTLSConfigForInternalPort loads the TLS config for the internal ports of the cluster
func LoadServerTLSConfigForInternalPort(v *viper.Viper) (*tls.Config, error) {
	if !v.GetBool("tls-internal-port-enabled") {
		return nil, nil
	}
	if v.GetString("tls-node-cert") == "" || v.GetString("tls-node-key") == "" {
		return nil, errors.Errorf("inter node tls is enabled but server node certs are not provided. " +
			"Please provide --tls_node_cert and --tls_node_key")
	}
	conf := TLSHelperConfig{}
	conf.UseSystemCACerts = v.GetBool("tls-use-system-ca")
	conf.RootCACert = v.GetString("tls-cacert")
	conf.CertRequired = true
	conf.Cert = v.GetString("tls-node-cert")
	conf.Key = v.GetString("tls-node-key")
	conf.ClientAuth = "REQUIREANDVERIFY"
	return GenerateServerTLSConfig(&conf)

}

// LoadServerTLSConfig loads the TLS config into the server with the given parameters.
func LoadServerTLSConfig(v *viper.Viper) (*tls.Config, error) {
	if v.GetString("tls-node-cert") == "" && v.GetString("tls-node-key") == "" {
		return nil, nil
	}

	conf := TLSHelperConfig{}
	conf.RootCACert = v.GetString("tls-cacert")
	conf.CertRequired = true
	conf.Cert = v.GetString("tls-node-cert")
	conf.Key = v.GetString("tls-node-key")
	conf.ClientAuth = v.GetString("tls-client-auth")
	conf.UseSystemCACerts = v.GetBool("tls-use-system-ca")
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
	}, nil
}

// LoadClientTLSConfig loads the TLS config into the client with the given parameters.
func LoadClientTLSConfig(v *viper.Viper) (*tls.Config, error) {
	if v.GetString("slash-grpc-endpoint") != "" {
		return SlashTLSConfig(v.GetString("slash-grpc-endpoint"))
	}

	// When the --tls-cacert option is pecified, the connection will be set up using TLS instead of
	// plaintext. However the client cert files are optional, depending on whether the server
	// requires a client certificate.
	caCert := v.GetString("tls-cacert")
	if caCert != "" {
		tlsCfg := tls.Config{}

		// 1. set up the root CA
		pool, err := generateCertPool(caCert, v.GetBool("tls-use-system-ca"))
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool

		// 2. set up the server name for verification
		tlsCfg.ServerName = v.GetString("tls-server-name")

		// 3. optionally load the client cert files
		certFile := v.GetString("tls-cert")
		keyFile := v.GetString("tls-key")
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
	if v.GetString("tls-server-name") != "" ||
		v.GetString("tls-cert") != "" ||
		v.GetString("tls-key") != "" {
		return nil, errors.Errorf("--tls-cacert is required for enabling TLS")
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

// GenerateServerTLSConfig creates and returns a new *tls.Config with the
// configuration provided.
func GenerateServerTLSConfig(config *TLSHelperConfig) (tlsCfg *tls.Config, err error) {
	if config.CertRequired {
		tlsCfg = new(tls.Config)
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

		return tlsCfg, nil
	}
	return nil, nil
}

// GenerateClientTLSConfig creates and returns a new client side *tls.Config with the
// configuration provided.
func GenerateClientTLSConfig(config *TLSHelperConfig) (tlsCfg *tls.Config, err error) {
	if config.CertRequired {
		tlsCfg := tls.Config{}
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
