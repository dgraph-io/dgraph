package admin

import (
	"context"
	"fmt"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

type updateGroupRewriter resolve.UpdateRewriter

func NewUpdateGroupRewriter() resolve.MutationRewriter {
	return &updateGroupRewriter{}
}

// Rewrite rewrites set and remove update patches into GraphQL+- upsert mutations
// only for Group type. It ensures that if a rule already exists in db, it is updated;
// otherwise, it is created. It also ensures that only the last rule out of all
// duplicate rules in input is preserved. A rule is duplicate if it has same predicate
// name as another rule.
func (urw *updateGroupRewriter) Rewrite(
	ctx context.Context,
	m schema.Mutation) ([]*resolve.UpsertMutation, error) {

	inp := m.ArgValue(schema.InputArgName).(map[string]interface{})
	setArg := inp["set"]
	delArg := inp["remove"]

	if setArg == nil && delArg == nil {
		return nil, nil
	}

	upsertQuery := resolve.RewriteUpsertQueryFromMutation(m, nil)
	srcUID := resolve.MutationQueryVarUID

	var errSet, errDel error
	var mutSet, mutDel []*dgoapi.Mutation
	varGen := resolve.NewVariableGenerator()
	ruleType := m.MutatedType().Field("rules").Type()

	if setArg != nil {
		rules, _ := setArg.(map[string]interface{})["rules"].([]interface{})
		rules, errs := removeDuplicateRuleRef(rules)
		if len(errs) != 0 {
			errSet = schema.GQLWrapf(errs, "failed to rewrite set payload")
		}
		for _, ruleI := range rules {
			rule := ruleI.(map[string]interface{})
			variable := varGen.Next(ruleType, "", "")
			predicate := rule["predicate"]
			permission := rule["permission"]

			addAclRuleQuery(upsertQuery, predicate.(string), variable)

			nonExistentJson := []byte(fmt.Sprintf(`
			{
				"uid": "%s",
				"dgraph.acl.rule": [
					{
						"uid":                    "_:%s",
						"dgraph.type":            "%s",
						"dgraph.rule.predicate":  "%s",
						"dgraph.rule.permission": %v
					}
				]
			}`, srcUID, variable, ruleType.DgraphName(), predicate, permission))

			existsJson := []byte(fmt.Sprintf(`
			{
				"uid":                    "uid(%s)",
				"dgraph.rule.permission": %v
			}`, variable, permission))

			mutSet = append(mutSet, &dgoapi.Mutation{
				SetJson: nonExistentJson,
				Cond: fmt.Sprintf(`@if(gt(len(%s),0) AND eq(len(%s),0))`, resolve.MutationQueryVar,
					variable),
			}, &dgoapi.Mutation{
				SetJson: existsJson,
				Cond: fmt.Sprintf(`@if(gt(len(%s),0) AND gt(len(%s),0))`, resolve.MutationQueryVar,
					variable),
			})
		}
	}

	if delArg != nil {
		rules, _ := delArg.(map[string]interface{})["rules"].([]interface{})
		var errs x.GqlErrorList
		for i, predicate := range rules {
			if predicate == "" {
				errs = appendEmptyPredicateError(errs, i)
				continue
			}

			variable := varGen.Next(ruleType, "", "")
			addAclRuleQuery(upsertQuery, predicate.(string), variable)

			deleteJson := []byte(fmt.Sprintf(`[
				{
					"uid": "%s",
					"dgraph.acl.rule": ["uid(%s)"]
				},
				{
					"uid": "uid(%s)"
				}
			]`, srcUID, variable, variable))

			mutDel = append(mutDel, &dgoapi.Mutation{
				DeleteJson: deleteJson,
				Cond: fmt.Sprintf(`@if(gt(len(%s),0) AND gt(len(%s),0))`, resolve.MutationQueryVar,
					variable),
			})
		}
		if len(errs) != 0 {
			errDel = schema.GQLWrapf(errs, "failed to rewrite remove payload")
		}
	}

	// if there is no mutation being performed as a result of some specific input,
	// then we don't need to do the upsertQuery for group
	if len(mutSet) == 0 && len(mutDel) == 0 {
		return nil, nil
	}

	return []*resolve.UpsertMutation{{
		Query:     &gql.GraphQuery{Children: []*gql.GraphQuery{upsertQuery}},
		Mutations: append(mutSet, mutDel...),
	}}, schema.GQLWrapf(schema.AppendGQLErrs(errSet, errDel), "failed to rewrite mutation payload")
}

// FromMutationResult rewrites the query part of a GraphQL update mutation into a Dgraph query.
func (urw *updateGroupRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return ((*resolve.UpdateRewriter)(urw)).FromMutationResult(ctx, mutation, assigned, result)
}

// addAclRuleQuery adds a *gql.GraphQuery to upsertQuery.Children to query a rule inside a group
// based on its predicate value.
func addAclRuleQuery(upsertQuery *gql.GraphQuery, predicate, variable string) {
	upsertQuery.Children = append(upsertQuery.Children, &gql.GraphQuery{
		Attr:  "dgraph.acl.rule",
		Alias: variable,
		Var:   variable,
		Filter: &gql.FilterTree{
			Op:    "",
			Child: nil,
			Func: &gql.Function{
				Name: "eq",
				Args: []gql.Arg{
					{
						Value: "dgraph.rule.predicate",
					},
					{
						Value: predicate,
					},
				},
			},
		},
	})
}
