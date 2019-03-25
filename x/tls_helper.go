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
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type tlsConfigType int8

const (
	TLSClientConfig tlsConfigType = iota
	TLSServerConfig
)

const (
	tlsRootCert = "ca.crt"
)

// TLSHelperConfig define params used to create a tls.Config
type TLSHelperConfig struct {
	ConfigType       tlsConfigType
	CertDir          string
	CertRequired     bool
	Cert             string
	Key              string
	ServerName       string
	RootCACert       string
	ClientAuth       string
	UseSystemCACerts bool
}

func RegisterTLSFlags(flag *pflag.FlagSet) {
	flag.String("tls_dir", "", "Path to directory that has TLS certificates and keys.")
	flag.Bool("tls_use_system_ca", true, "Include System CA into CA Certs.")
}

func LoadServerTLSConfig(v *viper.Viper, tlsCertFile string, tlsKeyFile string) (*tls.Config,
	error) {
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

func LoadClientTLSConfig(v *viper.Viper, tlsCertFile string, tlsKeyFile string) (*tls.Config,
	error) {
	conf := TLSHelperConfig{}
	conf.CertDir = v.GetString("tls_dir")
	// When the --tls_dir option is pecified, the connection will be set up using TLS instead of
	// plaintext, and hence there must be a ca.cert file under the directory for the client to
	// verify server's signature.
	// However the client cert files are optional, depending on whether the server is run
	// with the
	if conf.CertDir != "" {
		conf.CertRequired = true
		conf.RootCACert = path.Join(conf.CertDir, tlsRootCert)
		conf.Cert = path.Join(conf.CertDir, tlsCertFile)
		conf.Key = path.Join(conf.CertDir, tlsKeyFile)
		conf.UseSystemCACerts = v.GetBool("tls_use_system_ca")
		conf.ServerName = v.GetString("tls_server_name")

		return GenerateClientTLSConfig(&conf)
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
			return nil, fmt.Errorf("Error reading CA file '%s'.\n%s", certPath, err)
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
		return tls.NoClientCert, fmt.Errorf("Invalid client auth. Valid values [REQUEST, REQUIREANY, VERIFYIFGIVEN, REQUIREANDVERIFY]")
	}

	return tls.NoClientCert, nil
}

// GenerateTLSConfig creates and returns a new *tls.Config with the
// configuration provided. It returns a reload function if no problem is found.
// Otherwise an error is returned
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

		tlsCfg.MinVersion = tls.VersionTLS11
		tlsCfg.MaxVersion = tls.VersionTLS12

		return tlsCfg, nil
	}
	return nil, nil
}

// GenerateTLSConfig creates and returns a new client side *tls.Config with the
// configuration provided. If the ConfigType provided in TLSHelperConfig is
// TLSServerConfig, it's return a reload function. If any problem is found, an
// error is returned
func GenerateClientTLSConfig(config *TLSHelperConfig) (tlsCfg *tls.Config, err error) {
	pool, err := generateCertPool(config.RootCACert, config.UseSystemCACerts)
	if err != nil {
		return nil, err
	}

	return &tls.Config{RootCAs: pool, ServerName: config.ServerName}, nil
}
