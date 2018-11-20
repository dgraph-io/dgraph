package acl

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type AclUser struct {
	Userid   string
	Password string
}

func userAdd(dc *dgo.Dgraph) error {
	aclUser := AclUser{
		Userid:   UserAdd.Conf.GetString("user"),
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

func userDel(dc *dgo.Dgraph) error {
	userid := UserDel.Conf.GetString("user")
	// validate the userid
	if len(userid) == 0 {
		return fmt.Errorf("the user id should not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	dbUser, err := queryDBUser(txn, ctx, userid)
	if err != nil {
		return err
	}

	if dbUser == nil || len(dbUser.Uid) == 0 {
		glog.Infof("The user with id %v does not exist.", userid)
		return fmt.Errorf("Unable to delete user because it does not exist: %v", userid)
	}

	deleteUserNQuads := getDeleteUserOrGroupNQuads(dbUser.Uid)
	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		glog.Errorf("Unable to delete user: %v", err)
		return err
	}

	glog.Infof("Deleted user with id %v", userid)
	return nil
}

func userLogin(dc *dgo.Dgraph) error {
	aclUser := AclUser{
		Userid:   LogIn.Conf.GetString("user"),
		Password: LogIn.Conf.GetString("password"),
	}

	ctx := context.Background()
	glog.Infof("Logging with user:%+v", aclUser)
	err := dc.Login(ctx, aclUser.Userid, aclUser.Password)
	if err != nil {
		glog.Errorf("Unable to login:%v", err)
		return err
	}
	glog.Infof("Login successfully with jwt:\n%v", dc.GetJwt())
	return nil
}

type DBUser struct {
	Uid      string    `json:"uid"`
	UserID   string    `json:"dgraph.xid"`
	Password string    `json:"dgraph.password"`
	Groups   []DBGroup `json:"dgraph.user.group"`
}

func queryDBUser(txn *dgo.Txn, ctx context.Context, userid string) (dbUser *DBUser, err error) {
	queryUid := `
    query search($userid: string){
      user(func: eq(` + x.Acl_XId + `, $userid)) {
	    uid,
        ` + x.Acl_Password + `
        ` + x.Acl_UserGroup + ` {
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

	return &users[0], nil
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

	return createUserNQuads
}

func getDeleteUserOrGroupNQuads(xid string) []*api.NQuad {
	deleteUserNQuads := []*api.NQuad{
		{
			Subject:     xid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		}}

	return deleteUserNQuads
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
