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

var CmdAcl x.SubCommand
var UserAdd x.SubCommand
var UserDel x.SubCommand
var LogIn x.SubCommand

var GroupAdd x.SubCommand
var GroupDel x.SubCommand

var UserMod x.SubCommand
var ChMod x.SubCommand

const (
	tlsAclCert = "client.acl.crt"
	tlsAclKey  = "client.acl.key"
)

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := CmdAcl.Cmd.PersistentFlags()
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph gRPC server address")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")

	initSubcommands()

	var subcommands = []*x.SubCommand{
		&UserAdd, &UserDel, &LogIn, &GroupAdd, &GroupDel, &UserMod, &ChMod,
	}

	for _, sc := range subcommands {
		CmdAcl.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		sc.Conf.BindPFlags(sc.Cmd.Flags())
		sc.Conf.BindPFlags(CmdAcl.Cmd.PersistentFlags())
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

	// the usermod command used to set a user's groups
	UserMod.Cmd = &cobra.Command{
		Use:   "usermod",
		Short: "Run Dgraph acl tool to change a user's groups",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(UserMod.Conf, userMod)
		},
	}
	userModFlags := UserMod.Cmd.Flags()
	userModFlags.StringP("user", "u", "", "The user id to be changed")
	userModFlags.StringP("groups", "G", "", "The groups to be set for the user")

	// the chmod command is used to change a group's permissions
	ChMod.Cmd = &cobra.Command{
		Use:   "chmod",
		Short: "Run Dgraph acl tool to change a group's permissions",
		Run: func(cmd *cobra.Command, args []string) {
			runTxn(ChMod.Conf, chMod)
		},
	}
	chModFlags := ChMod.Cmd.Flags()
	chModFlags.StringP("group", "g", "", "The group whose permission is to be changed")
	chModFlags.StringP("pred", "p", "", "The predicates whose acls are to be changed")
	chModFlags.IntP("perm", "P", 0, "The acl represented using an integer, 4 for read-only, 2 for write-only, and 1 for modify-only")
}

func runTxn(conf *viper.Viper, f func(dgraph *dgo.Dgraph) error) {
	opt = options{
		dgraph: conf.GetString("dgraph"),
	}
	glog.Infof("Running transaction with dgraph endpoint: %v", opt.dgraph)

	if len(opt.dgraph) == 0 {
		glog.Fatalf("The --dgraph option must be set in order to connect to dgraph")
	}

	x.LoadTLSConfig(&tlsConf, CmdAcl.Conf, tlsAclCert, tlsAclKey)
	tlsConf.ServerName = CmdAcl.Conf.GetString("tls_server_name")

	ds := strings.Split(opt.dgraph, ",")
	var clients []api.DgraphClient
	var accessClients []api.DgraphAccessClient
	for _, d := range ds {
		conn, err := x.SetupConnection(d, !tlsConf.CertRequired, &tlsConf)
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
		glog.Errorf("Error while running transaction: %v", err)
		os.Exit(1)
	}
}
