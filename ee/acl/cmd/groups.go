package acl

import (
	"bytes"
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

	group, err := queryGroup(txn, ctx, groupId, []string{"uid"})
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

	group, err := queryGroup(txn, ctx, groupId, []string{"uid"})
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

func queryGroup(txn *dgo.Txn, ctx context.Context, groupid string, fields []string) (group *Group, err error) {
	var queryBuilder bytes.Buffer
	// write query header
	queryBuilder.WriteString(`    
      query search($groupid: string){
        group(func: eq(dgraph.xid, $groupid)) {
    `)

	for _, f := range fields {
		queryBuilder.WriteString(f)
		queryBuilder.WriteString("\n")
	}
	// write query footer
	queryBuilder.WriteString(`
      }
    }`)

	queryUid := queryBuilder.String()

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
		glog.Errorf("Unable to unmarshal the query group response:%v", err)
		return nil, err
	}
	groups := m[groupKey]
	if len(groups) == 0 {
		// the group does not exist
		return nil, nil
	}

	return &groups[0], nil
}

type Acl struct {
	Predicate string `json:"predicate"`
	Perm      uint32 `json:"perm"`
}

// parse the response and check existing of the uid
type Group struct {
	Uid     string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
	Acls    string `json:"dgraph.group.acl"`
}

func chMod(dc *dgo.Dgraph) error {
	groupId := ChMod.Conf.GetString("group")
	predicate := ChMod.Conf.GetString("pred")
	perm := uint32(ChMod.Conf.GetInt("perm"))
	if len(groupId) == 0 {
		return fmt.Errorf("the groupid must not be empty")
	}
	if len(predicate) == 0 {
		return fmt.Errorf("the predicate must not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	group, err := queryGroup(txn, ctx, groupId, []string{"uid", "dgraph.group.acl"})
	if err != nil {
		return err
	}

	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("Unable to change permission for group because it does not exist: %v", groupId)
	}

	currentAcls := []Acl{}
	if len(group.Acls) != 0 {
		if err := json.Unmarshal([]byte(group.Acls), &currentAcls); err != nil {
			return fmt.Errorf("Unable to unmarshal the acls associated with the group:%v", groupId)
		}
	}

	newAcls, updated := addAcl(currentAcls, &Acl{
		Predicate: predicate,
		Perm:      perm,
	})
	if !updated {
		glog.Infof("Nothing needs to be changed for the permission of group:%v", groupId)
		return nil
	}

	newAclBytes, err := json.Marshal(newAcls)
	if err != nil {
		return fmt.Errorf("Unable to marshal the updated acls:%v", err)
	}

	chModNQuads := &api.NQuad{
		Subject:     group.Uid,
		Predicate:   "dgraph.group.acl",
		ObjectValue: &api.Value{Val: &api.Value_BytesVal{BytesVal: newAclBytes}},
	}
	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{chModNQuads},
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to change mutations for the group %v on predicate %v: %v", groupId, predicate, err)
	}
	glog.Infof("Successfully changed permission for group %v on predicate %v to %v", groupId, predicate, perm)
	return nil
}

// returns whether the existing acls slice is changed
func addAcl(acls []Acl, newAcl *Acl) ([]Acl, bool) {
	for idx, acl := range acls {
		if acl.Predicate == newAcl.Predicate {
			if acl.Perm == newAcl.Perm {
				return acls, false
			}
			acls[idx].Perm = newAcl.Perm
			return acls, true
		}
	}

	// we do not find any existing acl matching the newAcl predicate
	return append(acls, *newAcl), true
}
