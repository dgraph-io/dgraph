package x

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
)

// TLSHelperConfig define params used to create a tls.Config
type TLSHelperConfig struct {
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

func setupCACert(caPool **x509.CertPool, certPath string, useSystemCA bool) error {
	if useSystemCA {
		var err error
		if *caPool, err = x509.SystemCertPool(); err != nil {
			return err
		}
	} else {
		*caPool = x509.NewCertPool()
	}

	if len(certPath) > 0 {
		caFile, err := ioutil.ReadFile(certPath)
		if err != nil {
			return err
		}

		if !(*caPool).AppendCertsFromPEM(caFile) {
			return fmt.Errorf("Error reading CA file '%s'.\n%s", certPath, err)
		}
	}

	return nil
}

func setupCertificate(cfg *tls.Config, required bool, certPath string, certKeyPath string, certKeyPass string) error {
	if len(certKeyPath) > 0 || len(certPath) > 0 || required {
		// Load key
		keyFile, err := ioutil.ReadFile(certKeyPath)
		if err != nil {
			return err
		}

		var certKey []byte
		if block, _ := pem.Decode(keyFile); block != nil {
			if x509.IsEncryptedPEMBlock(block) {
				decryptKey, err := x509.DecryptPEMBlock(block, []byte(certKeyPass))
				if err != nil {
					return err
				}

				privKey, err := x509.ParsePKCS1PrivateKey(decryptKey)
				if err != nil {
					return err
				}

				certKey = pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privKey),
				})
			} else {
				certKey = pem.EncodeToMemory(block)
			}
		} else {
			return fmt.Errorf("Invalid Cert Key")
		}

		// Load cert
		certFile, err := ioutil.ReadFile(certPath)
		if err != nil {
			return err
		}

		// Load certificate, pair cert/key
		certificate, err := tls.X509KeyPair(certFile, certKey)
		if err != nil {
			return fmt.Errorf("Error reading certificate {cert: '%s', key: '%s'}\n%s", certPath, certKeyPath, err)
		}

		cfg.Certificates = []tls.Certificate{certificate}
	}
	return nil
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

func setupClientAuth(cfg *tls.Config, authType string) error {
	auth := map[string]tls.ClientAuthType{
		"REQUEST":          tls.RequestClientCert,
		"REQUIREANY":       tls.RequireAnyClientCert,
		"VERIFYIFGIVEN":    tls.VerifyClientCertIfGiven,
		"REQUIREANDVERIFY": tls.RequireAndVerifyClientCert,
	}

	if len(authType) > 0 {
		if v, has := auth[strings.ToUpper(authType)]; has {
			cfg.ClientAuth = v
		} else {
			return fmt.Errorf("Invalid client auth. Valid values [REQUEST, REQUIREANY, VERIFYIFGIVEN, REQUIREANDVERIFY]")
		}
	}

	return nil
}

// GenerateTLSConfig creates and returns a new *tls.Config with the configuration provided, otherwise returns an error
func GenerateTLSConfig(config TLSHelperConfig) (*tls.Config, error) {
	var err error
	tlsCfg := &tls.Config{}

	err = setupCertificate(tlsCfg, config.CertRequired, config.Cert, config.Key, config.KeyPassphrase)
	if err != nil {
		return nil, err
	}

	// Configure Root CAs
	if len(config.RootCACerts) > 0 || config.UseSystemRootCACerts {
		err = setupCACert(&tlsCfg.RootCAs, config.RootCACerts, config.UseSystemRootCACerts)
		if err != nil {
			return nil, err
		}
	}

	// Configure Client CAs
	if len(config.ClientCACerts) > 0 || config.UseSystemClientCACerts {
		err = setupCACert(&tlsCfg.ClientCAs, config.ClientCACerts, config.UseSystemClientCACerts)
		if err != nil {
			return nil, err
		}
	}

	err = setupVersion(tlsCfg, config.MinVersion, config.MaxVersion)
	if err != nil {
		return nil, err
	}

	err = setupClientAuth(tlsCfg, config.ClientAuth)
	if err != nil {
		return nil, err
	}

	tlsCfg.InsecureSkipVerify = config.Insecure
	tlsCfg.ServerName = config.ServerName
	tlsCfg.BuildNameToCertificate()

	return tlsCfg, nil
}
