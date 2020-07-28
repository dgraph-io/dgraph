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
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgo/v200/protos/api"
)

const (
	queryACLGroupsBefore_v20_03_0 = `
		{
			rules(func: type(Group)) @filter(has(dgraph.group.acl)) {
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
	dg, conn, err := getDgoClient(true)
	if err != nil {
		return fmt.Errorf("error getting dgo client: %w", err)
	}
	defer conn.Close()

	data := make(map[string][]group)
	if err = getQueryResult(dg, queryACLGroupsBefore_v20_03_0, &data); err != nil {
		return fmt.Errorf("error querying old ACL rules: %w", err)
	}

	groups, ok := data["rules"]
	if !ok {
		return fmt.Errorf("unable to parse ACLs: %v", data)
	}

	counter := 1
	var nquads []*api.NQuad
	for _, group := range groups {
		if group.ACL == "" {
			continue
		}

		var rs rules
		if err := json.Unmarshal([]byte(group.ACL), &rs); err != nil {
			return fmt.Errorf("unable to unmarshal ACL: %v :: %w", string(group.ACL), err)
		}

		for _, r := range rs {
			newRuleStr := fmt.Sprintf("_:newrule%d", counter)
			nquads = append(nquads, []*api.NQuad{
				// the name of the type was Rule in v20.03.0
				getTypeNquad(newRuleStr, "Rule"),
				{
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
				},
			}...)

			counter++
		}
	}

	// Nothing to do.
	if len(nquads) == 0 {
		fmt.Println("nothing to do: no old rules found in the cluster")
		return nil
	}

	if err := mutateWithClient(dg, &api.Mutation{Set: nquads}); err != nil {
		return fmt.Errorf("error upgrading ACL rules: %w", err)
	}
	fmt.Println("Successfully upgraded ACL rules.")

	deleteOld := Upgrade.Conf.GetBool("deleteOld")
	if deleteOld {
		err = alterWithClient(dg, &api.Operation{
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
