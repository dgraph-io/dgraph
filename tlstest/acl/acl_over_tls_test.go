package acl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

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

func loadClientTLSConfig(v *viper.Viper) (*tls.Config, error) {
	// When the --tls_cacert option is pecified, the connection will be set up using TLS instead of
	// plaintext. However the client cert files are optional, depending on whether the server is
	// requiring a client certificate.
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
	}
	return nil, nil
}

func dgraphClientWithCerts(serviceAddr string, conf *viper.Viper) (*dgo.Dgraph, error) {
	tlsCfg, err := loadClientTLSConfig(conf)
	if err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{}
	if tlsCfg != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(serviceAddr, dialOpts...)
	if err != nil {
		return nil, err
	}
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	return dg, nil
}

func TestLoginOverTLS(t *testing.T) {
	conf := viper.New()
	conf.Set("tls_cacert", "../tls/ca.crt")
	conf.Set("tls_server_name", "node")

	dg, err := dgraphClientWithCerts(testutil.SockAddr, conf)
	if err != nil {
		t.Fatalf("Unable to get dgraph client: %s", err.Error())
	}
	for {
		err := dg.Login(context.Background(), "groot", "password")
		if err == nil {
			break
		} else if err != nil && !strings.Contains(err.Error(), "user not found") {
			t.Fatalf("Unable to login using the groot account: %v", err.Error())
		}

		time.Sleep(time.Second)
	}

	// Output:
}
