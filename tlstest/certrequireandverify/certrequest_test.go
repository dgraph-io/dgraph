package certrequireandverify

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestAccessWithoutClientCert(t *testing.T) {
	endpoint := ":9180"
	conf := viper.New()
	conf.Set("tls_cacert", "./tls/ca.crt")
	conf.Set("tls_server_name", "node")
	tlsCfg, err := x.LoadClientTLSConfig(conf)
	require.NoError(t, err, "Unable to load TLS config: %v", err)

	dialOpts := []grpc.DialOption{}
	if tlsCfg != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(endpoint, dialOpts...)
	require.NoError(t, err, "Unable to dial: %v", err)
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.Error(t, err, "The authentication handshake should have failed")
}

func TestAccessWithClientCert(t *testing.T) {
	endpoint := ":9180"
	conf := viper.New()
	conf.Set("tls_cacert", "./tls/ca.crt")
	conf.Set("tls_server_name", "node")
	conf.Set("tls_cert", "./tls/client.acl.crt")
	conf.Set("tls_key", "./tls/client.acl.key")
	tlsCfg, err := x.LoadClientTLSConfig(conf)
	require.NoError(t, err, "Unable to load TLS config: %v", err)

	dialOpts := []grpc.DialOption{}
	if tlsCfg != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(endpoint, dialOpts...)
	require.NoError(t, err, "Unable to dial: %v", err)
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err, "Unable to perform dropall: %v", err)
}
