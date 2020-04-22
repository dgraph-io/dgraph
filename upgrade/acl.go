/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
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
	ACL string `json:"dgraph.group.acl,omitempty"`
}

type rule struct {
	Predicate  string `json:"predicate,omitempty"`
	Permission int    `json:"perm,omitempty"`
}

type rules []rule

func upgradeACLRules() error {
	alpha := Upgrade.Conf.GetString("alpha")
	userName := Upgrade.Conf.GetString("user")
	password := Upgrade.Conf.GetString("password")
	deleteOld := Upgrade.Conf.GetBool("deleteOld")

	// TODO(Aman): add TLS configuration.
	conn, err := grpc.Dial(alpha, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("unable to connect to Dgraph cluster: %w", err)
	}
	defer conn.Close()

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// login to cluster
	if err := dg.Login(ctx, userName, password); err != nil {
		return fmt.Errorf("unable to login to Dgraph cluster: %w", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := dg.NewReadOnlyTxn().Query(ctx, oldACLQuery)
	if err != nil {
		return fmt.Errorf("unable to query old ACL rules: %w", err)
	}

	data := make(map[string][]group)
	if err := json.Unmarshal(resp.Json, &data); err != nil {
		return fmt.Errorf("unable to unmarshal old ACLs: %w", err)
	}

	groups, ok := data["rules"]
	if !ok {
		return fmt.Errorf("unable to parse ACLs: %v", string(resp.Json))
	}

	counter := 1
	var nquads []*api.NQuad
	for _, group := range groups {
		if len(group.ACL) == 0 {
			continue
		}

		var rs rules
		if err := json.Unmarshal([]byte(group.ACL), &rs); err != nil {
			return fmt.Errorf("unable to unmarshal ACL: %v :: %w", string(group.ACL), err)
		}

		for _, r := range rs {
			newRuleStr := fmt.Sprintf("_:newrule%d", counter)
			nquads = append(nquads, []*api.NQuad{{
				Subject:   newRuleStr,
				Predicate: "dgraph.rule.predicate",
				ObjectValue: &api.Value{
					Val: &api.Value_StrVal{StrVal: r.Predicate},
				},
			},
				{
					Subject:   newRuleStr,
					Predicate: "dgraph.rule.permission",
					ObjectValue: &api.Value{
						Val: &api.Value_IntVal{IntVal: int64(r.Permission)},
					},
				},
				{
					Subject:   group.UID,
					Predicate: "dgraph.acl.rule",
					ObjectId:  newRuleStr,
				}}...)

			counter++
		}
	}

	// Nothing to do.
	if len(nquads) == 0 {
		return fmt.Errorf("no old rules found in the cluster")
	}

	if err := mutateACL(dg, nquads); err != nil {
		return fmt.Errorf("error upgrading ACL rules: %w", err)
	}
	fmt.Println("Successfully upgraded ACL rules.")

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if deleteOld {
		err = dg.Alter(ctx, &api.Operation{
			DropOp:    api.Operation_ATTR,
			DropValue: "dgraph.group.acl",
		})
		if err != nil {
			return fmt.Errorf("error deleting old acl predicates: %w", err)
		}
		fmt.Println("Successfully deleted old rules.")
	}

	return nil
}

func mutateACL(dg *dgo.Dgraph, nquads []*api.NQuad) error {
	var err error
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

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
