package acl

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/spf13/viper"
)

func TestLoginOverTLS(t *testing.T) {
	conf := viper.New()
	conf.Set("tls-cacert", "../tls/alpha1/ca.crt")
	conf.Set("tls-server-name", "alpha1")

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)
	for {
		err := dg.Login(context.Background(), "groot", "password")
		if err == nil {
			break
		} else if err != nil && !strings.Contains(err.Error(), "user not found") {
			t.Fatalf("Unable to login using the groot account: %v", err.Error())
		}

		time.Sleep(time.Second)
	}
}
