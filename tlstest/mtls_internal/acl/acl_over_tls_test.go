package acl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoginOverTLS(t *testing.T) {
	conf := viper.New()
	conf.Set("tls-cacert", "../tls/alpha1/ca.crt")
	conf.Set("tls-server-name", "alpha1")

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)
	for i := 0; i < 30; i++ {
		err = dg.Login(context.Background(), "groot", "password")
		if err == nil {
			return
		}
		fmt.Printf("Login failed: %v. Retrying...\n", err)
		time.Sleep(time.Second)
	}

	t.Fatalf("Unable to login to %s\n", err)
}
