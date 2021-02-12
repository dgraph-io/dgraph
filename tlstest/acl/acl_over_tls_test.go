package acl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/viper"
)

func TestLoginOverTLS(t *testing.T) {
	conf := viper.New()
	conf.Set("tls_cacert", "../mtls_internal/tls/live/ca.crt")
	conf.Set("tls_server_name", "alpha1")

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)
	for i := 0; i < 30; i++ {
		err = dg.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace)
		if err == nil {
			return
		}
		fmt.Printf("Login failed: %v. Retrying...\n", err)
		time.Sleep(time.Second)
	}

	t.Fatalf("Unable to login to %s\n", err)
}
