package acl

import (
	"context"

	"github.com/dgraph-io/dgraph/z"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

func ExampleLoginOverTLS() {
	conf := viper.New()
	conf.Set("tls_cacert", "./tls/ca.crt")
	conf.Set("tls_server_name", "node")
	conf.Set("tls_server_name", "node")

	dg, err := z.DgraphClientWithCerts(":9180", conf)
	if err != nil {
		glog.Fatalf("Unable to get dgraph client: %v", err)
	}
	if err := dg.Login(context.Background(), "groot", "password"); err != nil {
		glog.Fatalf("Unable to login using the groot account: %v", err)
	}

	// Output:
}
