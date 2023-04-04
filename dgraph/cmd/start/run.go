package start

import (
	"crypto/tls"
	"time"

	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/spf13/cobra"
)

type options struct {
	raft              *z.SuperFlag
	telemetry         *z.SuperFlag
	limit             *z.SuperFlag
	bindall           bool
	portOffset        int
	numReplicas       int
	peer              string
	w                 string
	rebalanceInterval time.Duration
	tlsClientConfig   *tls.Config
	audit             *x.LoggerConf
	limiterConfig     *x.LimiterConf
}

var opts options

// Start is the sub-command used to start both Zero and Alpha in one process.
var Start x.SubCommand

func init() {
	Start.Cmd = &cobra.Command{
		Use:   "start",
		Short: "Run Dgraph Alpha and Zero management server ",
		Long: `
A Dgraph Zero instance manages the Dgraph cluster. A Dgraph Alpha instance stores the data.
This command merges them into
`,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Start.Conf).Stop()
			zero.Run(Start.Conf)
		},
		Annotations: map[string]string{"group": "core"},
	}
	// TODO Start.EnvPrefix = "DGRAPH_ZERO"
	Start.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Start.Cmd.Flags()
	x.FillCommonFlags(flag)
	// --tls SuperFlag
	x.RegisterServerTLSFlags(flag)

	zero.FillFlags(flag)
}
