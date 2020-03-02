package admin

import (
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type addGroupRewriter resolve.AddRewriter

func NewAddGroupRewriter() resolve.MutationRewriter {
	return &addGroupRewriter{}
}

// Rewrite rewrites schema.Mutation into GraphQL+- upsert mutations only for Group type.
// It ensures that only the last rule out of all duplicate rules in input is preserved.
// A rule is duplicate if it has same predicate name as another rule.
func (mrw *addGroupRewriter) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	addGroupInput, _ := m.ArgValue(schema.InputArgName).([]interface{})

	// remove rules with same predicate name for each group input
	for i, groupInput := range addGroupInput {
		rules, _ := groupInput.(map[string]interface{})["rules"].([]interface{})
		rules = removeDuplicateRuleRef(rules)
		addGroupInput[i].(map[string]interface{})["rules"] = rules
	}

	m.SetArgTo(schema.InputArgName, addGroupInput)

	return ((*resolve.AddRewriter)(mrw)).Rewrite(m)
}

// FromMutationResult rewrites the query part of a GraphQL add mutation into a Dgraph query.
func (mrw *addGroupRewriter) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return ((*resolve.AddRewriter)(mrw)).FromMutationResult(mutation, assigned, result)
}

// removeDuplicateRuleRef removes duplicate rules based on predicate value.
// for duplicate rules, only the last rule with duplicate predicate name is preserved.
func removeDuplicateRuleRef(rules []interface{}) []interface{} {
	predicateMap := make(map[string]int, len(rules))
	i := 0

	for _, rule := range rules {
		predicate, _ := rule.(map[string]interface{})["predicate"].(string)

		// this ensures that only the last rule with duplicate predicate name is preserved
		if idx, ok := predicateMap[predicate]; !ok {
			predicateMap[predicate] = i
			rules[i] = rule
			i++
		} else {
			rules[idx] = rule
		}
	}

	return rules[:i]
}
