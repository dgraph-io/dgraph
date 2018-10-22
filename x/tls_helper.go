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
	"sync"
	"time"

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
	tlsNodeCert = "node.crt"
	tlsNodeKey  = "node.key"
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

func LoadTLSConfig(conf *TLSHelperConfig, v *viper.Viper) {
	conf.CertDir = v.GetString("tls_dir")
	if conf.CertDir != "" {
		conf.CertRequired = true
		conf.RootCACert = path.Join(conf.CertDir, tlsRootCert)
		conf.Cert = path.Join(conf.CertDir, tlsNodeCert)
		conf.Key = path.Join(conf.CertDir, tlsNodeKey)
		conf.ClientAuth = v.GetString("tls_client_auth")
	}
	conf.UseSystemCACerts = v.GetBool("tls_use_system_ca")
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

func parseCertificate(required bool, certFile, keyFile string) (*tls.Certificate, error) {
	if !required {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &cert, nil
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
// configuration provided. If the ConfigType provided in TLSHelperConfig is
// TLSServerConfig, it's return a reload function. If any problem is found, an
// error is returned
func GenerateTLSConfig(config TLSHelperConfig) (tlsCfg *tls.Config, reloadConfig func(), err error) {
	wrapper := new(wrapperTLSConfig)
	tlsCfg = new(tls.Config)
	wrapper.config = tlsCfg

	cert, err := parseCertificate(config.CertRequired, config.Cert, config.Key)
	if err != nil {
		return nil, nil, err
	}

	if cert != nil {
		tlsCfg.Certificates = []tls.Certificate{*cert}
		pool, err := generateCertPool(config.RootCACert, config.UseSystemCACerts)
		if err != nil {
			return nil, nil, err
		}
		switch config.ConfigType {
		case TLSClientConfig:
			tlsCfg.RootCAs = pool

		case TLSServerConfig:
			wrapper.cert = &wrapperCert{cert: cert}
			tlsCfg.GetCertificate = wrapper.getCertificate
			tlsCfg.VerifyPeerCertificate = wrapper.verifyPeerCertificate
			tlsCfg.ClientCAs = pool
			wrapper.clientCAPool = &wrapperCAPool{pool: pool}
		}
	}

	auth, err := setupClientAuth(config.ClientAuth)
	if err != nil {
		return nil, nil, err
	}

	// If the client cert is required to be checked with the CAs
	if auth >= tls.VerifyClientCertIfGiven {
		// A custom cert validation is set because the current implementation is
		// not thread safe, it's needed bypass that validation and manually
		// manage the different cases, for that reason, the wrapper is
		// configured with the real auth level and the tlsCfg is only set with a
		// auth level who are a simile but without the use of any CA
		if auth == tls.VerifyClientCertIfGiven {
			tlsCfg.ClientAuth = tls.RequestClientCert
		} else {
			tlsCfg.ClientAuth = tls.RequireAnyClientCert
		}
		wrapper.clientAuth = auth
	} else {
		// it's not necessary a external validation with the CAs, so the wrapper
		// is not used
		tlsCfg.ClientAuth = auth
	}

	tlsCfg.MinVersion = tls.VersionTLS11
	tlsCfg.MaxVersion = tls.VersionTLS12
	tlsCfg.ServerName = config.ServerName

	if config.ConfigType == TLSClientConfig {
		return tlsCfg, nil, nil
	}

	wrapper.helperConfig = &config
	return tlsCfg, wrapper.reloadConfig, nil
}

type wrapperCert struct {
	sync.RWMutex
	cert *tls.Certificate
}

type wrapperCAPool struct {
	sync.RWMutex
	pool *x509.CertPool
}

type wrapperTLSConfig struct {
	mutex        sync.Mutex
	cert         *wrapperCert
	clientCert   *wrapperCert
	clientCAPool *wrapperCAPool
	clientAuth   tls.ClientAuthType
	config       *tls.Config
	helperConfig *TLSHelperConfig
}

func (c *wrapperTLSConfig) getCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.cert.RLock()
	cert := c.cert.cert
	c.cert.RUnlock()
	return cert, nil
}

func (c *wrapperTLSConfig) getClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	c.clientCert.RLock()
	cert := c.clientCert.cert
	c.clientCert.RUnlock()
	return cert, nil
}

func (c *wrapperTLSConfig) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if c.clientAuth >= tls.VerifyClientCertIfGiven && len(rawCerts) > 0 {
		if len(rawCerts) > 0 {
			pool := x509.NewCertPool()
			for _, raw := range rawCerts[1:] {
				if cert, err := x509.ParseCertificate(raw); err == nil {
					pool.AddCert(cert)
				} else {
					return Errorf("Invalid certificate")
				}
			}

			c.clientCAPool.RLock()
			clientCAs := c.clientCAPool.pool
			c.clientCAPool.RUnlock()
			opts := x509.VerifyOptions{
				Intermediates: pool,
				Roots:         clientCAs,
				CurrentTime:   time.Now(),
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}

			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			_, err = cert.Verify(opts)
			if err != nil {
				return Errorf("Failed to verify certificate")
			}
		} else {
			return Errorf("Invalid certificate")
		}
	}
	return nil
}

func (c *wrapperTLSConfig) reloadConfig() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Loading new certificate
	cert, err := parseCertificate(c.helperConfig.CertRequired, c.helperConfig.Cert, c.helperConfig.Key)
	if err != nil {
		fmt.Printf("Error reloading certificate. %v\nUsing current certificate\n", err)
	} else if cert != nil {
		if c.helperConfig.ConfigType == TLSServerConfig {
			c.cert.Lock()
			c.cert.cert = cert
			c.cert.Unlock()
		}
	}

	// Configure Client CAs
	if len(c.helperConfig.RootCACert) > 0 || c.helperConfig.UseSystemCACerts {
		pool, err := generateCertPool(c.helperConfig.RootCACert, c.helperConfig.UseSystemCACerts)
		if err != nil {
			fmt.Printf("Error reloading CAs. %v\nUsing current Client CAs\n", err)
		} else {
			c.clientCAPool.Lock()
			c.clientCAPool.pool = pool
			c.clientCAPool.Unlock()
		}
	}
}
