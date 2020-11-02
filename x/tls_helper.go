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
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	TLSNodeCert = "node.crt"
	TLSNodeKey  = "node.key"
)

const (
	tlsRootCert = "ca.crt"
)

// TLSHelperConfig define params used to create a tls.Config
type TLSHelperConfig struct {
	CertDir          string
	CertRequired     bool
	Cert             string
	Key              string
	ServerName       string
	RootCACert       string
	ClientAuth       string
	UseSystemCACerts bool
}

// RegisterClientTLSFlags registers the required flags to set up a TLS client.
func RegisterClientTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls_cacert", "",
		"The CA Cert file used to verify server certificates. Required for enabling TLS.")
	flag.Bool("tls_use_system_ca", true, "Include System CA into CA Certs.")
	flag.String("tls_server_name", "", "Used to verify the server hostname.")
	flag.String("tls_cert", "", "(optional) The Cert file provided by the client to the server.")
	flag.String("tls_key", "", "(optional) The private key file "+
		"provided by the client to the server.")
	flag.Bool("tls_internal_port_enabled", false, "enable inter node TLS encryption between cluster nodes.")
}

// LoadClientTLSConfigForInternalPort loads tls config for connecting to internal ports of cluster
func LoadClientTLSConfigForInternalPort(v *viper.Viper) (*tls.Config, error) {
	if !v.GetBool("tls_internal_port_enabled") {
		return nil, nil
	}
	conf := &TLSHelperConfig{}
	conf.UseSystemCACerts = true
	conf.CertDir = v.GetString("tls_dir")
	if conf.CertDir != "" {
		conf.CertRequired = true
		conf.RootCACert = path.Join(conf.CertDir, tlsRootCert)
		if v.GetString("tls_cert") == "" || v.GetString("tls_key") == "" {
			return nil, errors.Errorf("inter node tls is enabled but client certs are not provided. " +
				"Intern Node TLS is always client authenticated. Please provide --tls_cert and --tls_key")
		}

		conf.Cert = path.Join(conf.CertDir, v.GetString("tls_cert"))
		conf.Key = path.Join(conf.CertDir, v.GetString("tls_key"))
		return GenerateClientTLSConfig(conf)
	}

	// this is for clients which are defined via --tls_cacert
	conf.RootCACert = v.GetString("tls_cacert")
	if conf.RootCACert != "" {
		conf.CertRequired = true
		if v.GetString("tls_cert") == "" || v.GetString("tls_key") == "" {
			return nil, errors.Errorf("inter node tls is enabled but client certs are not provided. " +
				"Intern Node is TLS is always client authenticated. Please provide --tls_cert and --tls_key")
		}

		conf.Cert = v.GetString("tls_cert")
		conf.Key = v.GetString("tls_key")
		return GenerateClientTLSConfig(conf)
	}
	return nil, nil
}

// LoadServerTLSConfigForInternalPort loads the TLS config for the internal ports of the cluster
func LoadServerTLSConfigForInternalPort(tlsEnabled bool, tlsDir string) (*tls.Config, error) {
	if !tlsEnabled {
		return nil, nil
	}
	conf := TLSHelperConfig{}
	conf.CertDir = tlsDir
	conf.UseSystemCACerts = true
	if conf.CertDir != "" {
		conf.CertRequired = true
		conf.RootCACert = path.Join(conf.CertDir, tlsRootCert)
		conf.Cert = path.Join(conf.CertDir, TLSNodeCert)
		conf.Key = path.Join(conf.CertDir, TLSNodeKey)
		conf.ClientAuth = "REQUIREANDVERIFY"
		return GenerateServerTLSConfig(&conf)
	}

	return nil, nil
}

// LoadServerTLSConfig loads the TLS config into the server with the given parameters.
func LoadServerTLSConfig(v *viper.Viper, tlsCertFile string, tlsKeyFile string) (*tls.Config, error) {
	conf := TLSHelperConfig{}
	conf.CertDir = v.GetString("tls_dir")
	if conf.CertDir != "" {
		conf.CertRequired = true
		conf.RootCACert = path.Join(conf.CertDir, tlsRootCert)
		conf.Cert = path.Join(conf.CertDir, tlsCertFile)
		conf.Key = path.Join(conf.CertDir, tlsKeyFile)
		conf.ClientAuth = v.GetString("tls_client_auth")
	}
	conf.UseSystemCACerts = v.GetBool("tls_use_system_ca")

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
	if v.GetString("slash_grpc_endpoint") != "" {
		return SlashTLSConfig(v.GetString("slash_grpc_endpoint"))
	}

	// When the --tls_cacert option is pecified, the connection will be set up using TLS instead of
	// plaintext. However the client cert files are optional, depending on whether the server
	// requires a client certificate.
	caCert := v.GetString("tls_cacert")
	if caCert != "" {
		tlsCfg := tls.Config{}

		// 1. set up the root CA
		pool, err := generateCertPool(caCert, v.GetBool("tls_use_system_ca"))
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool

		// 2. set up the server name for verification
		tlsCfg.ServerName = v.GetString("tls_server_name")

		// 3. optionally load the client cert files
		certFile := v.GetString("tls_cert")
		keyFile := v.GetString("tls_key")
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
	if v.GetString("tls_server_name") != "" ||
		v.GetString("tls_cert") != "" ||
		v.GetString("tls_key") != "" {
		return nil, errors.Errorf("--tls_cacert is required for enabling TLS")
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
	caCert := config.RootCACert
	if caCert != "" {
		tlsCfg := tls.Config{}

		// 1. set up the root CA
		pool, err := generateCertPool(caCert, config.UseSystemCACerts)
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
