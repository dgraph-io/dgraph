package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"google.golang.org/grpc"
)

const (
	oldACLQuery = `
		{
			rules(func: type(Group)) {
				uid
				dgraph.group.acl
			}
		}
	`
)

type group struct {
	UID string `json:"uid"`
	ACL []byte `json:"dgraph.group.acl,omitempty"`
}

type rule struct {
	Predicate  string `json:"predicate,omitempty"`
	Permission int    `json:"perm,omitempty"`
}

type rules []rule

func main() {
	alpha := flag.String("a", "localhost:9180", "Alpha endpoint")
	userName := flag.String("u", "", "Username to login to Dgraph cluster")
	password := flag.String("p", "", "Password to login to Dgraph cluster")
	deleteOld := flag.Bool("d", false, "Delete the older ACL predicates")
	flag.Parse()

	// TODO: add TLS configuration.
	conn, err := grpc.Dial(*alpha, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("unable to connect to Dgraph cluster: %v\n", err)
		return
	}
	defer conn.Close()

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// login to cluster
	if err := dg.Login(ctx, *userName, *password); err != nil {
		fmt.Printf("unable to login to Dgraph cluster: %v\n", err)
		return
	}

	ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := dg.NewReadOnlyTxn().Query(ctx, oldACLQuery)
	if err != nil {
		fmt.Printf("unable to query old ACL rules: %v\n", err)
		return
	}

	data := make(map[string][]group)
	if err := json.Unmarshal(resp.GetJson(), &data); err != nil {
		fmt.Printf("unable to unmarshal old ACLs: %v\n", err)
		return
	}

	groups, ok := data["rules"]
	if !ok {
		fmt.Printf("Unable to parse ACLs: %v\n", string(resp.GetJson()))
		return
	}

	counter := 1
	var nquads []*api.NQuad
	for _, group := range groups {
		if len(group.ACL) == 0 {
			continue
		}

		var rs rules
		if err := json.Unmarshal(group.ACL, &rs); err != nil {
			fmt.Printf("Unable to unmarshal ACL: %v\n", string(group.ACL))
			return
		}

		for _, r := range rs {
			newRuleStr := fmt.Sprintf("_:newrule%d", counter)
			nquads = append(nquads, &api.NQuad{
				Subject:   newRuleStr,
				Predicate: "dgraph.rule.predicate",
				ObjectValue: &api.Value{
					Val: &api.Value_StrVal{StrVal: r.Predicate},
				},
			})

			nquads = append(nquads, &api.NQuad{
				Subject:   newRuleStr,
				Predicate: "dgraph.rule.permission",
				ObjectValue: &api.Value{
					Val: &api.Value_IntVal{IntVal: int64(r.Permission)},
				},
			})

			nquads = append(nquads, &api.NQuad{
				Subject:   group.UID,
				Predicate: "dgraph.acl.rule",
				ObjectId:  newRuleStr,
			})

			counter++
		}
	}

	ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := mutateACL(ctx, dg, nquads); err != nil {
		fmt.Printf("error upgrading ACL rules: %v\n", err)
		return
	}
	fmt.Println("Successfully upgraded ACL rules.")

	ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if *deleteOld {
		err = dg.Alter(ctx, &api.Operation{
			DropOp:    api.Operation_ATTR,
			DropValue: "dgraph.group.acl",
		})
		if err != nil {
			fmt.Printf("error deleting old acl predicates: %v\n", err)
			return
		}
		fmt.Println("Successfully deleted old rules.")
	}
}

func mutateACL(ctx context.Context, dg *dgo.Dgraph, nquads []*api.NQuad) error {
	var err error
	for i := 0; i < 3; i++ {
		_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
			Set:       nquads,
			CommitNow: true,
		})

		if err != nil {
			fmt.Printf("error in running mutation, retrying: %v\n", err)
			continue
		}

		return nil
	}

	return err
}
