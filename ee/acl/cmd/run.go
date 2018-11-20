package acl

import (
	"context"
	"encoding/json"
	"fmt"
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


type AclUser struct {
	Userid string
	Password string
}

func userAdd(dc *dgo.Dgraph) error {
	aclUser := AclUser{
		Userid: UserAdd.Conf.GetString("user"),
		Password: UserAdd.Conf.GetString("password"),
	}

	err := validateAclUser(&aclUser)
	if err != nil {
		glog.Errorf("Error while validating create user request: %v", err)
		return err
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	dbUser, err := queryDBUser(txn, ctx, aclUser.Userid)
	if err != nil {
		return err
	}

	if dbUser != nil {
		glog.Infof("The user with id %v already exists.", aclUser.Userid)
		return fmt.Errorf("Unable to create user because of conflict: %v", aclUser.Userid)
	}

	createUserNQuads := getCreateUserNQuads(aclUser.Userid, aclUser.Password)
	mu := &api.Mutation{
		CommitNow: true,
		Set:       createUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		glog.Errorf("Unable to create user: %v", err)
		return err
	}

	glog.Infof("Created new user with id %v", aclUser.Userid)
	return nil

}

// parse the response and check existing of the uid
type DBGroup struct {
	Uid string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
}

type DBUser struct {
	Uid string `json:"uid"`
	UserID string `json:"dgraph.xid"`
	Password string `json:"dgraph.password"`
	Groups []DBGroup `json:"dgraph.user.group"`
}

type QueryAgent interface {
	Query(ctx context.Context, req *api.Request) (*api.Response, error)
}

func queryDBUser(txn *dgo.Txn, ctx context.Context, userid string) (dbUser *DBUser, err error) {
	queryUid := `
    query search($userid: string){
      user(func: eq(` + x.Acl_XId + `, $userid)) {
	    uid,
        `+x.Acl_Password+`
        `+x.Acl_UserGroup+` {
          uid
          dgraph.xid
        }
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid

	queryResp, err := txn.QueryWithVars(ctx, queryUid, queryVars)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	dbUser, err = UnmarshallDBUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return dbUser, nil
}

// Extract the first DBUser pointed by the userKey in the query response
func UnmarshallDBUser(queryResp *api.Response, userKey string) (dbUser *DBUser, err error) {
	m := make(map[string][]DBUser)

	err = json.Unmarshal(queryResp.GetJson(), &m)
	if err != nil {
		glog.Errorf("Unable to unmarshal the query user response for user")
		return nil, err
	}
	users := m[userKey]
	if len(users) == 0 {
		// the user does not exist
		return nil, nil
	}

	dbUser = &users[0]
	return dbUser, nil
}

func getCreateUserNQuads(userid string, password string) []*api.NQuad {
	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_XId,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: userid}},
		},
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_Password,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: password}},
		}}

	// TODO: encode the user's attrs as a json blob and store under the x.Acl_UserBlob predicate
	return createUserNQuads
}

func validateAclUser(aclUser *AclUser) error {
	if len(aclUser.Userid) == 0 {
		return fmt.Errorf("The user must not be empty.")
	}
	if len(aclUser.Password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}
	return nil
}

