package acl

import (
	"fmt"
	"github.com/dgraph-io/dgraph/x"
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

func init() {
	Acl.Cmd = &cobra.Command{
		Use: "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := Acl.Cmd.Flags()
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
		sc.Conf.AutomaticEnv()
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
}

func initSubcommands() {
	// add sub commands under acl
	UserAdd.Cmd = &cobra.Command{
		Use:   "useradd",
		Short: "Run Dgraph acl tool to add a user",
		Run: func(cmd *cobra.Command, args []string) {
			userAdd()
		},
	}
	userAddFlags := UserAdd.Cmd.Flags()
	userAddFlags.StringP("user", "u", "", "The user id to be created")
	userAddFlags.StringP("password", "p", "", "The password for the user")
}

func run() {
	opt = options {
		dgraph : Acl.Conf.GetString("dgraph"),
	}
	x.LoadTLSConfig(&tlsConf, Acl.Conf)
	tlsConf.ServerName = Acl.Conf.GetString("tls_server_name")

	fmt.Printf("Running acl tool")
}

func userAdd() {
	fmt.Printf("Useradd")
}
/*
func (accessServer *AccessServer) CreateUser(ctx context.Context,
	request *api.CreateUserRequest) (*api.CreateUserResponse, error) {
	err := validateCreateUserRequest(request)
	if err != nil {
		glog.Errorf("Error while validating create user request: %v", err)
		return nil, err
	}

	// initiating a transaction on the server side
	txnContext := &api.TxnContext{}

	dbUser, err := queryDBUser(ctx, txnContext, request.User.Userid)
	if err != nil {
		return nil, err
	}

	resp := &api.CreateUserResponse{}
	if dbUser != nil {
		resp.Uid = dbUser.Uid
		resp.Code = api.AclResponseCode_CONFLICT
		return resp, nil
	}

	createUserNQuads := getCreateUserNQuads(request)
	mu := &api.Mutation{
		StartTs:   txnContext.StartTs, // required so that the query and mutation is run as a single transaction
		CommitNow: true,
		Set:       createUserNQuads,
	}

	assignedIds, err := (&edgraph.Server{}).Mutate(ctx, mu)
	if err != nil {
		glog.Errorf("Unable to create user: %v", err)
		return nil, err
	}
	dgo.MergeContext(txnContext, assignedIds.Context)

	resp.Uid = assignedIds.Uids[x.NewUserLabel]
	glog.Infof("Created new user with id %v", request.User.Userid)
	return resp, nil
}

func getCreateUserNQuads(request *api.CreateUserRequest) []*api.NQuad {
	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_XId,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: request.User.Userid}},
		},
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_Password,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: request.User.Password}},
		}}

	// TODO: encode the user's attrs as a json blob and store under the x.Acl_UserBlob predicate
	return createUserNQuads
}

func validateCreateUserRequest(request *api.CreateUserRequest) error {
	if len(request.User.Userid) == 0 {
		return fmt.Errorf("The userid must not be empty.")
	}
	if len(request.User.Password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}
	return nil
}
*/
