package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgo/v2/x"
	"google.golang.org/grpc"
)

type Rules []Rule

type Group struct {
	Uid   string `json:"uid"`
	Rules string `json:"dgraph.group.acl,omitempty"`
}

type Rule struct {
	Predicate  string `json:"predicate,omitempty"`
	Permission int    `json:"perm,omitempty"`
}

func main() {
	alpha := flag.String("a", "localhost:9180", "Alpha end point")
	userName := flag.String("u", "", "Username")
	password := flag.String("p", "", "Password")
	deleteOld := flag.Bool("d", false, "Delete the older ACL predicate.")
	flag.Parse()

	conn, err := grpc.Dial(*alpha, grpc.WithInsecure())
	x.Check(err)
	defer conn.Close()

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	err = dg.Login(ctx, *userName, *password)
	x.Check(err)

	query := `
	{
		me(func: type(Group)) {
			uid
			dgraph.group.acl
		}
	}
	`

	ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	x.Check(err)

	data := make(map[string][]Group)
	err = json.Unmarshal(resp.GetJson(), &data)
	x.Check(err)

	groups, ok := data["me"]
	if !ok {
		fmt.Errorf("Unable to parse ACLs: %+v", string(resp.GetJson()))
		os.Exit(1)
	}

	ruleCount := 1
	var nquads []*api.NQuad
	for _, group := range groups {
		var rules Rules
		if group.Rules == "" {
			continue
		}

		err = json.Unmarshal([]byte(group.Rules), &rules)
		x.Check(err)
		for _, rule := range rules {
			newRuleStr := fmt.Sprintf("_:newrule%d", ruleCount)
			nquads = append(nquads, &api.NQuad{
				Subject:   newRuleStr,
				Predicate: "dgraph.rule.predicate",
				ObjectValue: &api.Value{
					Val: &api.Value_StrVal{StrVal: rule.Predicate},
				},
			})
			nquads = append(nquads, &api.NQuad{
				Subject:   newRuleStr,
				Predicate: "dgraph.rule.permission",
				ObjectValue: &api.Value{
					Val: &api.Value_IntVal{IntVal: int64(rule.Permission)},
				},
			})
			nquads = append(nquads, &api.NQuad{
				Subject:   group.Uid,
				Predicate: "dgraph.acl.rule",
				ObjectId:  newRuleStr,
			})

			ruleCount++
		}
	}

	ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	if !mutateACL(ctx, &api.Mutation{Set: nquads, CommitNow: true}, dg, nquads) {
		fmt.Println("Error Restoring ACL rules.")
	}

	ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	if *deleteOld {
		err = dg.Alter(ctx, &api.Operation{
			DropOp:    api.Operation_ATTR,
			DropValue: "dgraph.group.acl",
		})
		if err != nil {
			fmt.Println(err)
			fmt.Println("Error deleting old acl predicate.")
		}
	}
}

func mutateACL(ctx context.Context, mutation *api.Mutation, dg *dgo.Dgraph,
	nquads []*api.NQuad) bool {
	var err error
	for i := 0; i < 3; i++ {
		_, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
			Set:       nquads,
			CommitNow: true,
		})

		if err != nil {
			continue
		}
		fmt.Println("Successfully restored ACL rules.")
		return true
	}

	fmt.Println(err)
	return false
}
