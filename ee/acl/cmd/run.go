package acl

import (
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

type options struct {
	dgraph string
}

var opt options
var tlsConf x.TLSHelperConfig

var Acl x.SubCommand
var UserAdd x.SubCommand

const (
	tlsAclCert = "client.acl.crt"
	tlsAclKey  = "client.acl.key"
)

func init() {
	Acl.Cmd = &cobra.Command{
		Use: "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := Acl.Cmd.PersistentFlags()
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph gRPC server address")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")

	initSubcommands()

	var subcommands = []*x.SubCommand {
		&UserAdd,
	}

	for _, sc := range subcommands {
		Acl.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		sc.Conf.BindPFlags(sc.Cmd.Flags())
		sc.Conf.BindPFlags(Acl.Cmd.PersistentFlags())
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
}

func initSubcommands() {
	// add sub commands under acl
	UserAdd.Cmd = &cobra.Command{
		Use:   "useradd",
		Short: "Run Dgraph acl tool to add a user",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(UserAdd.Conf, userAdd)
		},
	}
	userAddFlags := UserAdd.Cmd.Flags()
	userAddFlags.StringP("user", "u", "", "The user id to be created")
	userAddFlags.StringP("password", "p", "", "The password for the user")
}

func runTxn(conf *viper.Viper, f func(dgraph *dgo.Dgraph) error) {
	opt = options{
		dgraph: conf.GetString("dgraph"),
	}

	if len(opt.dgraph) == 0 {
		glog.Fatalf("The --dgraph option must be set in order to connect to dgraph")
	}

	ds := strings.Split(opt.dgraph, ",")
	var clients []api.DgraphClient
	for _, d := range ds {
		conn, err := x.SetupConnection(d, !tlsConf.CertRequired, &tlsConf, tlsAclCert, tlsAclKey)
		x.Checkf(err, "While trying to setup connection to Dgraph alpha.")
		defer conn.Close()

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}
	dgraphClient := dgo.NewDgraphClient(clients...)
	f(dgraphClient)
}



// parse the response and check existing of the uid
type DBGroup struct {
	Uid string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
}

