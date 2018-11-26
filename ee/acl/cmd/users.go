package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func userAdd(dc *dgo.Dgraph) error {
	userid := UserAdd.Conf.GetString("user")
	password := UserAdd.Conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty.")
	}
	if len(password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(txn, ctx, userid)
	if err != nil {
		return err
	}

	if user != nil {
		return fmt.Errorf("Unable to create user because of conflict: %v", userid)
	}

	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: userid}},
		},
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: password}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to create user: %v", err)
	}

	glog.Infof("Created new user with id %v", userid)
	return nil
}

func userDel(dc *dgo.Dgraph) error {
	userid := UserDel.Conf.GetString("user")
	// validate the userid
	if len(userid) == 0 {
		return fmt.Errorf("The user id should not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(txn, ctx, userid)
	if err != nil {
		return err
	}

	if user == nil || len(user.Uid) == 0 {
		return fmt.Errorf("Unable to delete user because it does not exist: %v", userid)
	}

	deleteUserNQuads := []*api.NQuad{
		{
			Subject:     user.Uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to delete user: %v", err)
	}

	glog.Infof("Deleted user with id %v", userid)
	return nil
}

func userLogin(dc *dgo.Dgraph) error {
	userid := LogIn.Conf.GetString("user")
	password := LogIn.Conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty.")
	}
	if len(password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}

	ctx := context.Background()

	err := dc.Login(ctx, userid, password)
	if err != nil {
		return fmt.Errorf("Unable to login:%v", err)
	}
	glog.Infof("Login successfully with jwt:\n%v", dc.GetJwt())
	return nil
}

type User struct {
	Uid           string  `json:"uid"`
	UserID        string  `json:"dgraph.xid"`
	Password      string  `json:"dgraph.password"`
	PasswordMatch bool    `json:"password_match"`
	Groups        []Group `json:"dgraph.user.group"`
}

func queryUser(txn *dgo.Txn, ctx context.Context, userid string) (user *User, err error) {
	queryUid := `
    query search($userid: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid

	queryResp, err := txn.QueryWithVars(ctx, queryUid, queryVars)
	if err != nil {
		return nil, fmt.Errorf("Error while query user with id %s: %v", userid, err)
	}
	user, err = UnmarshallUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

// Extract the first User pointed by the userKey in the query response
func UnmarshallUser(queryResp *api.Response, userKey string) (user *User, err error) {
	m := make(map[string][]User)

	err = json.Unmarshal(queryResp.GetJson(), &m)
	if err != nil {
		return nil, fmt.Errorf("Unable to unmarshal the query user response for user:%v", err)
	}
	users := m[userKey]
	if len(users) == 0 {
		// the user does not exist
		return nil, nil
	}

	return &users[0], nil
}

func userMod(dc *dgo.Dgraph) error {
	userid := UserMod.Conf.GetString("user")
	groups := UserMod.Conf.GetString("groups")
	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(txn, ctx, userid)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("The user does not exist: %v", userid)
	}

	targetGroupsMap := make(map[string]bool)
	if len(groups) > 0 {
		for _, g := range strings.Split(groups, ",") {
			targetGroupsMap[g] = true
		}
	}

	existingGroupsMap := make(map[string]bool)
	for _, g := range user.Groups {
		existingGroupsMap[g.GroupID] = true
	}
	newGroups, groupsToBeDeleted := calcDiffs(targetGroupsMap, existingGroupsMap)

	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{},
		Del:       []*api.NQuad{},
	}

	for _, g := range newGroups {
		glog.Infof("Adding user %v to group %v", userid, g)
		nquad, err := getUserModNQuad(txn, ctx, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}

		mu.Set = append(mu.Set, nquad)
	}

	for _, g := range groupsToBeDeleted {
		glog.Infof("Deleting user %v from group %v", userid, g)
		nquad, err := getUserModNQuad(txn, ctx, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}
		mu.Del = append(mu.Del, nquad)
	}
	if len(mu.Del) == 0 && len(mu.Set) == 0 {
		glog.Infof("Nothing nees to be changed for the groups of user:%v", userid)
		return nil
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return err
	}

	glog.Infof("Successfully modifed groups for user %v", userid)
	return nil
}

func getUserModNQuad(txn *dgo.Txn, ctx context.Context, useruid string, groupid string) (*api.NQuad, error) {
	group, err := queryGroup(txn, ctx, groupid)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("The group does not exist:%v", groupid)
	}

	createUserGroupNQuads := &api.NQuad{
		Subject:   useruid,
		Predicate: "dgraph.user.group",
		ObjectId:  group.Uid,
	}

	return createUserGroupNQuads, nil
}

func calcDiffs(targetMap map[string]bool, existingMap map[string]bool) ([]string, []string) {

	newGroups := []string{}
	groupsToBeDeleted := []string{}

	for g, _ := range targetMap {
		if _, ok := existingMap[g]; !ok {
			newGroups = append(newGroups, g)
		}
	}
	for g, _ := range existingMap {
		if _, ok := targetMap[g]; !ok {
			groupsToBeDeleted = append(groupsToBeDeleted, g)
		}
	}

	return newGroups, groupsToBeDeleted
}
