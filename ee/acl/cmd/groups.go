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

func groupAdd(dc *dgo.Dgraph) error {
	groupId := GroupAdd.Conf.GetString("group")
	if len(groupId) == 0 {
		return fmt.Errorf("the group id should not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	group, err := queryGroup(txn, ctx, groupId)
	if err != nil {
		return err
	}

	if group != nil {
		return fmt.Errorf("The group with id %v already exists.", groupId)
	}

	createGroupNQuads := []*api.NQuad{
		{
			Subject:     "_:newgroup",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: groupId}},
		},
	}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createGroupNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to create user: %v", err)
	}

	glog.Infof("Created new group with id %v", groupId)
	return nil
}

func groupDel(dc *dgo.Dgraph) error {
	groupId := GroupDel.Conf.GetString("group")
	if len(groupId) == 0 {
		return fmt.Errorf("the group id should not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	group, err := queryGroup(txn, ctx, groupId)
	if err != nil {
		return err
	}

	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("Unable to delete group because it does not exist: %v", groupId)
	}

	deleteGroupNQuads := []*api.NQuad{
		{
			Subject:     group.Uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		}}
	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteGroupNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to delete group: %v", err)
	}

	glog.Infof("Deleted user with id %v", groupId)
	return nil
}

func queryGroup(txn *dgo.Txn, ctx context.Context, groupid string) (group *Group, err error) {
	queryUid := `
    query search($groupid: string){
      group(func: eq(dgraph.xid, $groupid)) {
	    uid,
        dgraph.xid
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$groupid"] = groupid

	queryResp, err := txn.QueryWithVars(ctx, queryUid, queryVars)
	if err != nil {
		glog.Errorf("Error while query group with id %s: %v", groupid, err)
		return nil, err
	}
	group, err = UnmarshallGroup(queryResp, "group")
	if err != nil {
		return nil, err
	}
	return group, nil
}

// Extract the first User pointed by the userKey in the query response
func UnmarshallGroup(queryResp *api.Response, groupKey string) (group *Group, err error) {
	m := make(map[string][]Group)

	err = json.Unmarshal(queryResp.GetJson(), &m)
	if err != nil {
		glog.Errorf("Unable to unmarshal the query group response")
		return nil, err
	}
	groups := m[groupKey]
	if len(groups) == 0 {
		// the group does not exist
		return nil, nil
	}

	return &groups[0], nil
}

// parse the response and check existing of the uid
type Group struct {
	Uid     string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
}

func chMod(dc *dgo.Dgraph) error {
	groupId := ChMod.Conf.GetString("group")
	predicate := ChMod.Conf.GetString("predicate")
	acl := ChMod.Conf.GetInt64("acl")
	if len(groupId) == 0 {
		return fmt.Errorf("the groupid must not be empty")
	}
	if len(predicate) == 0 {
		return fmt.Errorf("the predicate must not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	group, err := queryGroup(txn, ctx, groupId)
	if err != nil {
		return err
	}

	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("Unable to change permission for group because it does not exist: %v", groupId)
	}
	chModNQuads := &api.NQuad{
		Subject:     group.Uid,
		Predicate:   predicate,
		ObjectValue: &api.Value{Val: &api.Value_IntVal{IntVal: acl}},
	}
	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{chModNQuads},
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("unable to change mutations for the group %v on predicate %v: %v", groupId, predicate, err)
	}
	glog.Infof("Successfully changed permission for group %v on predicate %v to %v", groupId, predicate, acl)
	return nil
}
