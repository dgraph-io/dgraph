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

package schema

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cast"

	"github.com/dgraph-io/dgraph/gql"

	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/gqlerror"
	"github.com/dgraph-io/gqlparser/v2/parser"
	"github.com/dgraph-io/gqlparser/v2/validator"
)

const (
	RBACQueryPrefix = "{"
)

type RBACQuery struct {
	Variable string
	Operator string
	Operand  interface{}
	regex    *regexp.Regexp
}

type RuleNode struct {
	Or        []*RuleNode
	And       []*RuleNode
	Not       *RuleNode
	Rule      Query
	DQLRule   *gql.GraphQuery
	RBACRule  *RBACQuery
	Variables ast.VariableDefinitionList
}

type AuthContainer struct {
	Password *RuleNode
	Query    *RuleNode
	Add      *RuleNode
	Update   *RuleNode
	Delete   *RuleNode
}

type RuleResult int

const (
	Uncertain RuleResult = iota
	Positive
	Negative
)

func checkIfExistInArray(arr []interface{}, val interface{}) RuleResult {
	for _, v := range arr {
		if reflect.DeepEqual(v, val) {
			return Positive
		}
	}
	return Negative
}

func checkIfEqual(val1 interface{}, val2 interface{}) RuleResult {
	if reflect.DeepEqual(val1, val2) {
		return Positive
	}
	return Negative
}

func checkIfRegexMatchExistInArray(regex *regexp.Regexp, arr []interface{}) RuleResult {
	for _, v := range arr {
		strv, ok := v.(string)
		if ok && regex.MatchString(strv) {
			return Positive
		}
	}
	return Negative
}

func checkIfRegexMatch(regex *regexp.Regexp, val1 interface{}) RuleResult {
	strv, ok := val1.(string)
	if ok && regex.MatchString(strv) {
		return Positive
	}
	return Negative
}

// EvaluateRBACRule evaluates the auth token based on the auth query
// There are two cases here:
// 1. Auth token has an array of values for the variable.
// 2. Auth token has non-array value for the variable.
// match would be deep equal except for regex match in case of regexp operator.
// In case array one match would made the rule positive.
// For example, Rule {$USER: { eq:"uid"}} and token $USER:["u", "id", "uid"] would result in match.
// also Rule {$USER: { in: ["uid", "xid"]}} and token $USER:["u", "id", "uid"] would also result in match.
func (rq *RBACQuery) EvaluateRBACRule(av map[string]interface{}) RuleResult {
	switch rq.Operator {
	case "eq":
		// if eq, auth rule value will be matched completely
		tokenValues, err := cast.ToSliceE(av[rq.Variable])
		if err != nil {
			// this means value for variable in token in not an array
			return checkIfEqual(av[rq.Variable], rq.Operand)
		}
		return checkIfExistInArray(tokenValues, rq.Operand)
	case "regexp":
		// if regexp, auth rule value should always be string and so as token values
		tokenValues, err := cast.ToSliceE(av[rq.Variable])
		if err != nil {
			// this means value for variable in token in not an array
			return checkIfRegexMatch(rq.regex, av[rq.Variable])
		}
		return checkIfRegexMatchExistInArray(rq.regex, tokenValues)

	case "in":
		// if in, auth rule will only have array as the value
		ruleOperand := cast.ToSlice(rq.Operand)
		tokenValues, err := cast.ToSliceE(av[rq.Variable])
		if err != nil {
			// this means value for variable in token in not an array
			return checkIfExistInArray(ruleOperand, av[rq.Variable])
		}

		for _, t := range tokenValues {
			if checkIfExistInArray(ruleOperand, t) == Positive {
				return Positive
			}
		}
	}
	return Negative
}

func (node *RuleNode) staticEvaluation(av map[string]interface{}) RuleResult {
	for _, v := range node.Variables {
		if _, ok := av[v.Variable]; !ok {
			return Negative
		}
	}
	return Uncertain
}

func (node *RuleNode) EvaluateStatic(av map[string]interface{}) RuleResult {
	if node == nil {
		return Uncertain
	}

	hasUncertain := false
	for _, rule := range node.Or {
		val := rule.EvaluateStatic(av)
		if val == Positive {
			return Positive
		} else if val == Uncertain {
			hasUncertain = true
		}
	}

	if len(node.Or) > 0 && !hasUncertain {
		return Negative
	}

	for _, rule := range node.And {
		val := rule.EvaluateStatic(av)
		if val == Negative {
			return Negative
		} else if val == Uncertain {
			hasUncertain = true
		}
	}

	if len(node.And) > 0 && !hasUncertain {
		return Positive
	}

	if node.Not != nil {
		// In the case of a non-RBAC query, the result indicates whether the query has all the
		// variables in order to evaluate it. Hence, we don't need to negate the value.
		result := node.Not.EvaluateStatic(av)
		if node.Not.RBACRule == nil {
			return result
		}
		switch result {
		case Uncertain:
			return Uncertain
		case Positive:
			return Negative
		case Negative:
			return Positive
		}
	}

	if node.RBACRule != nil {
		return node.RBACRule.EvaluateRBACRule(av)
	}

	if node.Rule != nil {
		return node.staticEvaluation(av)
	}
	return Uncertain
}

type TypeAuth struct {
	Rules  *AuthContainer
	Fields map[string]*AuthContainer
}

func createEmptyDQLRule(typeName string) *RuleNode {
	return &RuleNode{DQLRule: &gql.GraphQuery{
		Attr: typeName + "Root",
		Var:  typeName + "Root",
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: typeName}},
		},
	},
	}
}

func authRules(s *ast.Schema) (map[string]*TypeAuth, error) {
	//TODO: Add position in error.
	var errResult, err error
	authRules := make(map[string]*TypeAuth)

	for _, typ := range s.Types {
		name := typeName(typ)
		authRules[name] = &TypeAuth{Fields: make(map[string]*AuthContainer)}
		auth := typ.Directives.ForName(authDirective)
		if auth != nil {
			authRules[name].Rules, err = parseAuthDirective(s, typ, auth)
			errResult = AppendGQLErrs(errResult, err)
		}

		for _, field := range typ.Fields {
			auth := field.Directives.ForName(authDirective)
			if auth != nil {
				authRules[name].Fields[field.Name], err = parseAuthDirective(s, typ, auth)
				errResult = AppendGQLErrs(errResult, err)
			}
		}
	}

	// Merge the Auth rules on interfaces into the implementing types
	for _, typ := range s.Types {
		name := typeName(typ)
		if typ.Kind == ast.Object {
			for _, intrface := range typ.Interfaces {
				interfaceName := typeName(s.Types[intrface])
				if authRules[interfaceName] != nil && authRules[interfaceName].Rules != nil {
					authRules[name].Rules = mergeAuthRules(authRules[name].Rules, authRules[interfaceName].Rules, mergeAuthNodeWithAnd)
				}
			}
		}
	}

	// Reinitialize the Interface's auth to be empty as Any operation on interface
	// will be broken into an operation on subsequent implementing types and auth rules
	// will be verified against the types only.
	for _, typ := range s.Types {
		name := typeName(typ)
		if typ.Kind == ast.Interface {
			authRules[name] = &TypeAuth{}
		}
	}

	return authRules, errResult
}

func mergeAuthNodeWithOr(objectAuth, interfaceAuth *RuleNode) *RuleNode {
	if objectAuth == nil {
		return interfaceAuth
	}

	if interfaceAuth == nil {
		return objectAuth
	}

	ruleNode := &RuleNode{}
	ruleNode.Or = append(ruleNode.Or, objectAuth, interfaceAuth)
	return ruleNode
}

func mergeAuthNodeWithAnd(objectAuth, interfaceAuth *RuleNode) *RuleNode {
	if objectAuth == nil {
		return interfaceAuth
	}

	if interfaceAuth == nil {
		return objectAuth
	}

	ruleNode := &RuleNode{}
	ruleNode.And = append(ruleNode.And, objectAuth, interfaceAuth)
	return ruleNode
}

func mergeAuthRules(objectAuthRules, interfaceAuthRules *AuthContainer, mergeAuthNode func(*RuleNode, *RuleNode) *RuleNode) *AuthContainer {
	// return copy of interfaceAuthRules since it is a pointer and otherwise it will lead
	// to unnecessary errors
	if objectAuthRules == nil {
		return &AuthContainer{
			Password: interfaceAuthRules.Password,
			Query:    interfaceAuthRules.Query,
			Add:      interfaceAuthRules.Add,
			Delete:   interfaceAuthRules.Delete,
			Update:   interfaceAuthRules.Update,
		}
	}

	objectAuthRules.Password = mergeAuthNode(objectAuthRules.Password, interfaceAuthRules.Password)
	objectAuthRules.Query = mergeAuthNode(objectAuthRules.Query, interfaceAuthRules.Query)
	objectAuthRules.Add = mergeAuthNode(objectAuthRules.Add, interfaceAuthRules.Add)
	objectAuthRules.Delete = mergeAuthNode(objectAuthRules.Delete, interfaceAuthRules.Delete)
	objectAuthRules.Update = mergeAuthNode(objectAuthRules.Update, interfaceAuthRules.Update)
	return objectAuthRules
}

func parseAuthDirective(
	s *ast.Schema,
	typ *ast.Definition,
	dir *ast.Directive) (*AuthContainer, error) {

	if dir == nil || len(dir.Arguments) == 0 {
		return nil, nil
	}

	var errResult, err error
	result := &AuthContainer{}

	if pwd := dir.Arguments.ForName("password"); pwd != nil && pwd.Value != nil {
		result.Password, err = parseAuthNode(s, typ, pwd.Value)
		errResult = AppendGQLErrs(errResult, err)
	}

	if qry := dir.Arguments.ForName("query"); qry != nil && qry.Value != nil {
		result.Query, err = parseAuthNode(s, typ, qry.Value)
		errResult = AppendGQLErrs(errResult, err)
	}

	if add := dir.Arguments.ForName("add"); add != nil && add.Value != nil {
		result.Add, err = parseAuthNode(s, typ, add.Value)
		errResult = AppendGQLErrs(errResult, err)
	}

	if upd := dir.Arguments.ForName("update"); upd != nil && upd.Value != nil {
		result.Update, err = parseAuthNode(s, typ, upd.Value)
		errResult = AppendGQLErrs(errResult, err)
	}

	if del := dir.Arguments.ForName("delete"); del != nil && del.Value != nil {
		result.Delete, err = parseAuthNode(s, typ, del.Value)
		errResult = AppendGQLErrs(errResult, err)
	}

	return result, errResult
}

func parseAuthNode(s *ast.Schema, typ *ast.Definition, val *ast.Value) (*RuleNode, error) {

	if len(val.Children) == 0 {
		return nil, gqlerror.Errorf("Type %s: @auth: no arguments - "+
			"there should be only one of \"and\", \"or\", \"not\" and \"rule\"", typ.Name)
	}

	numChildren := 0
	var errResult error
	result := &RuleNode{}

	if ors := val.Children.ForName("or"); ors != nil && len(ors.Children) > 0 {
		for _, or := range ors.Children {
			rn, err := parseAuthNode(s, typ, or.Value)
			result.Or = append(result.Or, rn)
			errResult = AppendGQLErrs(errResult, err)
		}
		if len(result.Or) < 2 {
			errResult = AppendGQLErrs(errResult, gqlerror.Errorf(
				`Type %s: @auth: 'OR' should contain at least two rules`, typ.Name))
		}
		numChildren++
	}

	if ands := val.Children.ForName("and"); ands != nil && len(ands.Children) > 0 {
		for _, and := range ands.Children {
			rn, err := parseAuthNode(s, typ, and.Value)
			result.And = append(result.And, rn)
			errResult = AppendGQLErrs(errResult, err)
		}
		if len(result.And) < 2 {
			errResult = AppendGQLErrs(errResult, gqlerror.Errorf(
				`Type %s: @auth: 'AND' should contain at least two rules`, typ.Name))
		}
		numChildren++
	}

	if not := val.Children.ForName("not"); not != nil &&
		len(not.Children) == 1 && not.Children[0] != nil {

		var err error
		result.Not, err = parseAuthNode(s, typ, not)
		errResult = AppendGQLErrs(errResult, err)
		numChildren++
	}

	if rule := val.Children.ForName("rule"); rule != nil {
		var err error
		if strings.HasPrefix(rule.Raw, RBACQueryPrefix) {
			result.RBACRule, err = rbacValidateRule(typ, rule.Raw)
		} else {
			err = gqlValidateRule(s, typ, rule.Raw, result)
		}
		errResult = AppendGQLErrs(errResult, err)
		numChildren++
	}

	if numChildren != 1 || len(val.Children) > 1 {
		errResult = AppendGQLErrs(errResult, gqlerror.Errorf("Type %s: @auth: there "+
			"should be only one of \"and\", \"or\", \"not\" and \"rule\"", typ.Name))
	}

	return result, errResult
}

func rbacValidateRule(typ *ast.Definition, rule string) (*RBACQuery, error) {
	rbacRegex, err :=
		regexp.Compile(`^{[\s]?(.*?)[\s]?:[\s]?{[\s]?(\w*)[\s]?:[\s]?(.*)[\s]?}[\s]?}$`)
	if err != nil {
		return nil, gqlerror.Errorf("Type %s: @auth: `%s` error while parsing rule.",
			typ.Name, err)
	}

	idx := rbacRegex.FindAllStringSubmatchIndex(rule, -1)
	if len(idx) != 1 || len(idx[0]) != 8 || rule != rule[idx[0][0]:idx[0][1]] {
		return nil, gqlerror.Errorf("Type %s: @auth: `%s` is not a valid rule.",
			typ.Name, rule)
	}
	//	bool, for booleans
	//	float64, for numbers
	//	string, for strings
	//	[]interface{}, for JSON arrays
	//	map[string]interface{}, for JSON objects
	//	nil for JSON null
	var op interface{}
	if err = json.Unmarshal([]byte(rule[idx[0][6]:idx[0][7]]), &op); err != nil {
		return nil, gqlerror.Errorf("Type %s: @auth: `%s` is not a valid GraphQL variable.",
			typ.Name, rule[idx[0][2]:idx[0][3]])
	}

	//objects and nil values are not supported in rules
	if op == nil {
		return nil, gqlerror.Errorf("Type %s: @auth: `%s` is not a valid GraphQL variable. "+
			"object or nil values aren't supported", typ.Name, rule[idx[0][2]:idx[0][3]])
	}
	query := RBACQuery{
		Variable: rule[idx[0][2]:idx[0][3]],
		Operator: rule[idx[0][4]:idx[0][5]],
		Operand:  op,
	}

	if !strings.HasPrefix(query.Variable, "$") {
		return nil, gqlerror.Errorf("Type %s: @auth: `%s` is not a valid GraphQL variable.",
			typ.Name, query.Variable)
	}
	query.Variable = query.Variable[1:]
	if err = validateRBACOperators(typ, query); err != nil {
		return nil, err
	}

	if query.Operator == "regexp" {
		regex, err := regexp.Compile(query.Operand.(string))
		if err != nil {
			return nil, err
		}
		query.regex = regex
	}

	return &query, nil
}

func validateRBACOperators(typ *ast.Definition, query RBACQuery) error {
	var ok = false
	switch query.Operator {
	case "eq": // only check we could have is for primitive but not necessary we will have equal match later
		ok = true
	case "regexp":
		_, ok = query.Operand.(string)
	case "in":
		// auth rule value should be of array type
		_, ok = query.Operand.([]interface{})
	default:
		return gqlerror.Errorf("Type %s: @auth: `%s` operator is not supported in "+
			"this rule.", typ.Name, query.Operator)
	}
	if !ok {
		return gqlerror.Errorf("Type %s: @auth: `%s` operator has invalid value in "+
			"this rule.", typ.Name, query.Operator)
	}
	return nil
}

func gqlValidateRule(s *ast.Schema, typ *ast.Definition, rule string, node *RuleNode) error {
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: rule})
	if gqlErr != nil {
		return gqlerror.Errorf("Type %s: @auth: failed to parse GraphQL rule "+
			"[reason : %s]", typ.Name, gqlErr.Message)
	}

	if len(doc.Operations) != 1 {
		return gqlerror.Errorf("Type %s: @auth: a rule should be "+
			"exactly one query, found %v GraphQL operations", typ.Name, len(doc.Operations))
	}

	op := doc.Operations[0]
	if op == nil {
		return gqlerror.Errorf("Type %s: @auth: a rule should be "+
			"exactly one query, found an empty GraphQL operation", typ.Name)
	}

	if op.Operation != "query" {
		return gqlerror.Errorf("Type %s: @auth: a rule should be exactly"+
			" one query, found an %s", typ.Name, op.Name)
	}

	listErr := validator.Validate(s, doc)
	if len(listErr) != 0 {
		var errs error
		for _, err := range listErr {
			errs = AppendGQLErrs(errs, gqlerror.Errorf("Type %s: @auth: failed to "+
				"validate GraphQL rule [reason : %s]", typ.Name, err.Message))
		}
		return errs
	}

	if len(op.SelectionSet) != 1 {
		return gqlerror.Errorf("Type %s: @auth: a rule should be exactly one "+
			"query, found %v queries", typ.Name, len(op.SelectionSet))
	}

	f, ok := op.SelectionSet[0].(*ast.Field)
	if !ok {
		return gqlerror.Errorf("Type %s: @auth: error couldn't generate query from rule",
			typ.Name)
	}

	if f.Name != "query"+typ.Name {
		return gqlerror.Errorf("Type %s: @auth: expected only query%s "+
			"rules,but found %s", typ.Name, typ.Name, f.Name)
	}

	node.Rule = &query{
		field: f,
		op: &operation{op: op,
			query: rule,
			doc:   doc,
			// need to fill in vars and schema at query time
		},
		sel: op.SelectionSet[0]}
	node.Variables = op.VariableDefinitions
	return nil
}
