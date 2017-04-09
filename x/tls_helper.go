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

package x

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"
)

type tlsConfigType int8

const (
	TLSClientConfig tlsConfigType = iota
	TLSServerConfig
)

// TLSHelperConfig define params used to create a tls.Config
type TLSHelperConfig struct {
	ConfigType             tlsConfigType
	CertRequired           bool
	Cert                   string
	Key                    string
	KeyPassphrase          string
	ServerName             string
	Insecure               bool
	RootCACerts            string
	UseSystemRootCACerts   bool
	ClientAuth             string
	ClientCACerts          string
	UseSystemClientCACerts bool
	MinVersion             string
	MaxVersion             string
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

func parseCertificate(required bool, certPath string, certKeyPath string, certKeyPass string) (*tls.Certificate, error) {
	if len(certKeyPath) > 0 || len(certPath) > 0 || required {
		// Load key
		keyFile, err := ioutil.ReadFile(certKeyPath)
		if err != nil {
			return nil, err
		}

		var certKey []byte
		if block, _ := pem.Decode(keyFile); block != nil {
			if x509.IsEncryptedPEMBlock(block) {
				decryptKey, err := x509.DecryptPEMBlock(block, []byte(certKeyPass))
				if err != nil {
					return nil, err
				}

				privKey, err := x509.ParsePKCS1PrivateKey(decryptKey)
				if err != nil {
					return nil, err
				}

				certKey = pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privKey),
				})
			} else {
				certKey = pem.EncodeToMemory(block)
			}
		} else {
			return nil, fmt.Errorf("Invalid Cert Key")
		}

		// Load cert
		certFile, err := ioutil.ReadFile(certPath)
		if err != nil {
			return nil, err
		}

		// Load certificate, pair cert/key
		certificate, err := tls.X509KeyPair(certFile, certKey)
		if err != nil {
			return nil, fmt.Errorf("Error reading certificate {cert: '%s', key: '%s'}\n%s", certPath, certKeyPath, err)
		}

		return &certificate, nil
	}
	return nil, nil
}

func setupVersion(cfg *tls.Config, minVersion string, maxVersion string) error {
	// Configure TLS version
	tlsVersion := map[string]uint16{
		"TLS11": tls.VersionTLS11,
		"TLS12": tls.VersionTLS12,
	}

	if len(minVersion) > 0 {
		if val, has := tlsVersion[strings.ToUpper(minVersion)]; has {
			cfg.MinVersion = val
		} else {
			return fmt.Errorf("Invalid min_version '%s'. Valid values [TLS11, TLS12]", minVersion)
		}
	} else {
		cfg.MinVersion = tls.VersionTLS11
	}

	if len(maxVersion) > 0 {
		if val, has := tlsVersion[strings.ToUpper(maxVersion)]; has && val >= cfg.MinVersion {
			cfg.MaxVersion = val
		} else {
			if has {
				return fmt.Errorf("Cannot use '%s' as max_version, it's lower than '%s'", maxVersion, minVersion)
			}
			return fmt.Errorf("Invalid max_version '%s'. Valid values [TLS11, TLS12]", maxVersion)
		}
	} else {
		cfg.MaxVersion = tls.VersionTLS12
	}
	return nil
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

	cert, err := parseCertificate(config.CertRequired, config.Cert, config.Key, config.KeyPassphrase)
	if err != nil {
		return nil, nil, err
	}

	if cert != nil {
		if config.ConfigType == TLSClientConfig {
			tlsCfg.Certificates = []tls.Certificate{*cert}
			tlsCfg.BuildNameToCertificate()
		}

		if config.ConfigType == TLSServerConfig {
			wrapper.cert = &wrapperCert{cert: cert}
			tlsCfg.GetCertificate = wrapper.getCertificate
			tlsCfg.VerifyPeerCertificate = wrapper.verifyPeerCertificate
		}
	}

	auth, err := setupClientAuth(config.ClientAuth)
	if err != nil {
		return nil, nil, err
	}

	if auth >= tls.VerifyClientCertIfGiven {
		if auth == tls.VerifyClientCertIfGiven {
			tlsCfg.ClientAuth = tls.RequestClientCert
		} else {
			tlsCfg.ClientAuth = tls.RequireAnyClientCert
		}
		wrapper.clientAuth = auth
	} else {
		tlsCfg.ClientAuth = auth
	}

	// Configure Root CAs
	if len(config.RootCACerts) > 0 || config.UseSystemRootCACerts {
		pool, err := generateCertPool(config.RootCACerts, config.UseSystemRootCACerts)
		if err != nil {
			return nil, nil, err
		}
		tlsCfg.RootCAs = pool
	}

	// Configure Client CAs
	if len(config.ClientCACerts) > 0 || config.UseSystemClientCACerts {
		pool, err := generateCertPool(config.ClientCACerts, config.UseSystemClientCACerts)
		if err != nil {
			return nil, nil, err
		}
		tlsCfg.ClientCAs = x509.NewCertPool()
		wrapper.clientCAPool = &wrapperCAPool{pool: pool}
	}

	err = setupVersion(tlsCfg, config.MinVersion, config.MaxVersion)
	if err != nil {
		return nil, nil, err
	}

	tlsCfg.InsecureSkipVerify = config.Insecure
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
	if c.clientAuth == tls.VerifyClientCertIfGiven && len(rawCerts) == 0 {
		return nil
	} else if c.clientAuth >= tls.VerifyClientCertIfGiven {
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
				Roots:         clientCAs,
				CurrentTime:   time.Now(),
				Intermediates: pool,
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
	cert, err := parseCertificate(c.helperConfig.CertRequired, c.helperConfig.Cert, c.helperConfig.Key, c.helperConfig.KeyPassphrase)
	if err != nil {
		log.Printf("Error reloading certificate. %s\nUsing current certificate\n", err.Error())
	} else if cert != nil {
		if c.helperConfig.ConfigType == TLSServerConfig {
			c.cert.Lock()
			c.cert.cert = cert
			c.cert.Unlock()
		}
	}

	// Configure Client CAs
	if len(c.helperConfig.ClientCACerts) > 0 || c.helperConfig.UseSystemClientCACerts {
		pool, err := generateCertPool(c.helperConfig.ClientCACerts, c.helperConfig.UseSystemClientCACerts)
		if err != nil {
			log.Printf("Error reloading CAs. %s\nUsing current Client CAs\n", err.Error())
		} else {
			c.clientCAPool.Lock()
			c.clientCAPool.pool = pool
			c.clientCAPool.Unlock()
		}
	}
}
