package acl

import (
	"os"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type options struct {
	dgraph string
}

var opt options
var tlsConf x.TLSHelperConfig

var Acl x.SubCommand
var UserAdd x.SubCommand
var UserDel x.SubCommand
var LogIn x.SubCommand

var GroupAdd x.SubCommand
var GroupDel x.SubCommand

const (
	tlsAclCert = "client.acl.crt"
	tlsAclKey  = "client.acl.key"
)

func init() {
	Acl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := Acl.Cmd.PersistentFlags()
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph gRPC server address")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")

	initSubcommands()

	var subcommands = []*x.SubCommand{
		&UserAdd, &UserDel, &LogIn, &GroupAdd, &GroupDel,
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
	// user creation command
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

	// user deletion command
	UserDel.Cmd = &cobra.Command{
		Use:   "userdel",
		Short: "Run Dgraph acl tool to delete a user",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(UserDel.Conf, userDel)
		},
	}
	userDelFlags := UserDel.Cmd.Flags()
	userDelFlags.StringP("user", "u", "", "The user id to be deleted")

	// login command
	LogIn.Cmd = &cobra.Command{
		Use:   "login",
		Short: "Login to dgraph in order to get a jwt token",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(LogIn.Conf, userLogin)
		},
	}
	loginFlags := LogIn.Cmd.Flags()
	loginFlags.StringP("user", "u", "", "The user id to be created")
	loginFlags.StringP("password", "p", "", "The password for the user")

	// group creation command
	GroupAdd.Cmd = &cobra.Command{
		Use:   "groupadd",
		Short: "Run Dgraph acl tool to add a group",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(GroupAdd.Conf, groupAdd)
		},
	}
	groupAddFlags := GroupAdd.Cmd.Flags()
	groupAddFlags.StringP("group", "g", "", "The group id to be created")

	// group deletion command
	GroupDel.Cmd = &cobra.Command{
		Use:   "groupdel",
		Short: "Run Dgraph acl tool to delete a group",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(GroupDel.Conf, groupDel)
		},
	}
	groupDelFlags := GroupDel.Cmd.Flags()
	groupDelFlags.StringP("group", "g", "", "The group id to be deleted")

}

func runTxn(conf *viper.Viper, f func(dgraph *dgo.Dgraph) error) {
	opt = options{
		dgraph: conf.GetString("dgraph"),
	}
	glog.Infof("running transaction with dgraph endpoint: %v", opt.dgraph)

	if len(opt.dgraph) == 0 {
		glog.Fatalf("The --dgraph option must be set in order to connect to dgraph")
	}

	ds := strings.Split(opt.dgraph, ",")
	var clients []api.DgraphClient
	var accessClients []api.DgraphAccessClient
	for _, d := range ds {
		conn, err := x.SetupConnection(d, !tlsConf.CertRequired, &tlsConf, tlsAclCert, tlsAclKey)
		x.Checkf(err, "While trying to setup connection to Dgraph alpha.")
		defer conn.Close()

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)

		dgraphAccess := api.NewDgraphAccessClient(conn)
		accessClients = append(accessClients, dgraphAccess)
	}
	dgraphClient := dgo.NewDgraphClient(clients...)
	dgraphClient.SetAccessClients(accessClients...)
	if err := f(dgraphClient); err != nil {
		glog.Errorf("error while running transaction: %v", err)
		os.Exit(1)
	}
}

// parse the response and check existing of the uid
type DBGroup struct {
	Uid     string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
}
