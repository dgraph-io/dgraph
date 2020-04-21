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
	"regexp"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

const (
	RBACQueryPrefix = "{"
)

type RBACQuery struct {
	Variable string
	Operator string
	Operand  string
}

type AuthQuery struct {
	query *query
	rbac  *RBACQuery
	name  string

	isRbac bool
}

func (aq *AuthQuery) GetName() string {
	if aq.isRbac {
		return aq.rbac.Operand
	}

	return ""
}

func (aq *AuthQuery) GetQuery(av map[string]interface{}) *query {
	q := aq.query
	return &query{
		field: (*field)(q).field,
		op: &operation{op: q.op.op,
			query:    q.op.query,
			doc:      q.op.doc,
			inSchema: q.op.inSchema,
			vars:     av,
		},
		sel: q.sel}
}

func (aq *AuthQuery) IsDeepQuery() bool {
	if aq.isRbac {
		return false
	}

	for _, q := range aq.query.SelectionSet() {
		if len(q.SelectionSet()) > 0 {
			return true
		}
	}
	return false
}

func (aq *AuthQuery) IsRBAC() bool {
	return aq.isRbac
}

func (rc *RBACQuery) Evaluate(a *AuthState) bool {
	if rc.Operator == "eq" {
		return a.AuthVariables[rc.Variable] == rc.Operand
	}

	return false
}

func (aq *AuthQuery) EvaluateRBACRule(a *AuthState) RuleResult {
	if !aq.isRbac {
		return Uncertain
	}

	if aq.rbac.Evaluate(a) {
		return Positive
	}
	return Negative
}

type RuleNode struct {
	Or   []*RuleNode
	And  []*RuleNode
	Not  *RuleNode
	Rule *AuthQuery

	RuleID   int
	RuleName string
}

type RuleResult int

const (
	Uncertain RuleResult = iota
	Positive
	Negative
)

type AuthContainer struct {
	Query  *RuleNode
	Add    *RuleNode
	Update *RuleNode
	Delete *RuleNode
}

type TypeAuth struct {
	rules  *AuthContainer
	fields map[string]*AuthContainer
}

var currRule = 0

func (r *RuleNode) IsRBAC() bool {
	for _, i := range r.Or {
		if i.IsRBAC() {
			return true
		}
	}
	for _, i := range r.And {
		if i.IsRBAC() {
			return true
		}
	}

	if r.Not != nil && r.Not.IsRBAC() {
		return true
	}

	if r.Rule != nil {
		return r.Rule.IsRBAC()
	}

	return false
}

func (node *RuleNode) GetRuleID() int {
	return node.RuleID
}

func (node *RuleNode) GetRBACRules(a *AuthState) {
	a.RbacRule[node.RuleID] = Uncertain
	valid := Uncertain
	for _, rule := range node.Or {
		if rule.IsRBAC() {
			rule.GetRBACRules(a)
			if a.RbacRule[rule.RuleID] == Positive {
				valid = Positive
			}
		}
		a.RbacRule[node.RuleID] = valid
	}

	for _, rule := range node.And {
		if rule.IsRBAC() {
			rule.GetRBACRules(a)
			if a.RbacRule[rule.RuleID] == Negative {
				valid = Negative
			}
		}
		a.RbacRule[node.RuleID] = valid
	}

	if node.Not != nil && node.Not.IsRBAC() {
		node.Not.GetRBACRules(a)
		childValue := a.RbacRule[node.Not.RuleID]
		switch childValue {

		case Uncertain:
			a.RbacRule[node.RuleID] = Uncertain
		case Positive:
			a.RbacRule[node.RuleID] = Negative
		case Negative:
			a.RbacRule[node.RuleID] = Positive
		}

	}

	if node.Rule != nil && node.Rule.IsRBAC() {
		a.RbacRule[node.RuleID] = node.Rule.EvaluateRBACRule(a)
	}
}

func authRules(s *ast.Schema, dgraphPredicate map[string]map[string]string,
	typeNameAst map[string][]*ast.Definition) (map[string]*TypeAuth, error) {

	var errResult, err error
	authRules := make(map[string]*TypeAuth)

	for _, typ := range s.Types {
		currRule = 0
		name := typeName(typ)
		authRules[name] = &TypeAuth{fields: make(map[string]*AuthContainer)}
		auth := typ.Directives.ForName(authDirective)
		if auth != nil {
			authRules[name].rules, err = parseAuthDirective(s, typ, auth,
				dgraphPredicate, typeNameAst)
			errResult = AppendGQLErrs(errResult, err)
		}

		for _, field := range typ.Fields {
			auth := field.Directives.ForName(authDirective)
			if auth != nil {
				authRules[name].fields[field.Name], err = parseAuthDirective(s,
					typ, auth, dgraphPredicate, typeNameAst)
				errResult = AppendGQLErrs(errResult, err)
			}
		}
	}

	return authRules, errResult
}

func parseAuthDirective(
	s *ast.Schema,
	typ *ast.Definition,
	dir *ast.Directive,
	dgraphPredicate map[string]map[string]string,
	typeNameAst map[string][]*ast.Definition) (*AuthContainer, error) {

	if dir == nil || len(dir.Arguments) == 0 {
		return nil, nil
	}

	var errResult, err error
	result := &AuthContainer{}

	if qry := dir.Arguments.ForName("query"); qry != nil && qry.Value != nil {
		result.Query, err = parseAuthNode(s, typ, qry.Value, dgraphPredicate, typeNameAst)
		errResult = AppendGQLErrs(errResult, err)
	}

	if add := dir.Arguments.ForName("add"); add != nil && add.Value != nil {
		result.Add, err = parseAuthNode(s, typ, add.Value, dgraphPredicate, typeNameAst)
		errResult = AppendGQLErrs(errResult, err)
	}

	if upd := dir.Arguments.ForName("update"); upd != nil && upd.Value != nil {
		result.Update, err = parseAuthNode(s, typ, upd.Value, dgraphPredicate, typeNameAst)
		errResult = AppendGQLErrs(errResult, err)
	}

	if del := dir.Arguments.ForName("delete"); del != nil && del.Value != nil {
		result.Delete, err = parseAuthNode(s, typ, del.Value, dgraphPredicate, typeNameAst)
		errResult = AppendGQLErrs(errResult, err)
	}

	return result, errResult
}

func parseAuthNode(s *ast.Schema, typ *ast.Definition, val *ast.Value,
	dgraphPredicate map[string]map[string]string,
	typeNameAst map[string][]*ast.Definition) (*RuleNode, error) {
	if len(val.Children) == 0 {
		return nil,
			gqlerror.ErrorPosf(val.Position,
				`no arguments - there should be only one of "and", "or", "not" and "rule"`)
	}

	numChildren := 0
	var errResult error
	currRule++
	result := &RuleNode{RuleID: currRule, RuleName: typ.Name}

	if ors := val.Children.ForName("or"); ors != nil && len(ors.Children) > 0 {
		for _, or := range ors.Children {
			rn, err := parseAuthNode(s, typ, or.Value, dgraphPredicate, typeNameAst)
			result.Or = append(result.Or, rn)
			errResult = AppendGQLErrs(errResult, err)
		}
		if len(result.Or) < 2 {
			errResult = AppendGQLErrs(errResult,
				gqlerror.ErrorPosf(ors.Position,
					`it doesn't make sense to 'OR' less than two rules`))
		}
		numChildren++
	}

	if ands := val.Children.ForName("and"); ands != nil && len(ands.Children) > 0 {
		for _, and := range ands.Children {
			rn, err := parseAuthNode(s, typ, and.Value, dgraphPredicate, typeNameAst)
			result.And = append(result.And, rn)
			errResult = AppendGQLErrs(errResult, err)
		}
		if len(result.And) < 2 {
			errResult = AppendGQLErrs(errResult,
				gqlerror.ErrorPosf(ands.Position,
					`it doesn't make sense to 'And' less than two rules`))
		}
		numChildren++
	}

	if not := val.Children.ForName("not"); not != nil &&
		len(not.Children) == 1 && not.Children[0] != nil {

		var err error
		result.Not, err = parseAuthNode(s, typ, not, dgraphPredicate, typeNameAst)
		errResult = AppendGQLErrs(errResult, err)
		numChildren++
	}

	if rule := val.Children.ForName("rule"); rule != nil {
		var err error
		aq := &AuthQuery{}
		if strings.HasPrefix(rule.Raw, RBACQueryPrefix) {
			aq.rbac, err = rbacValidateRule(typ, rule.Raw, rule.Position)
			aq.isRbac = true
		} else {
			aq.isRbac = false
			aq.query, err = gqlValidateRule(s, typ, rule.Raw, rule.Position,
				dgraphPredicate, typeNameAst)
		}
		result.Rule = aq
		errResult = AppendGQLErrs(errResult, err)
		numChildren++
	}

	if numChildren != 1 || len(val.Children) > 1 {
		errResult = AppendGQLErrs(errResult,
			gqlerror.ErrorPosf(val.Position,
				`there should be only one of "and", "or", "not" and "rule"`))
	}

	return result, errResult
}

func rbacValidateRule(typ *ast.Definition, rule string,
	position *ast.Position) (*RBACQuery, error) {
	rbacRegex, err :=
		regexp.Compile(`^{[\s]?(.*?)[\s]?:[\s]?{[\s]?(\w*)[\s]?:[\s]?"(.*)"[\s]?}[\s]?}$`)
	if err != nil {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: `%s` error while parsing auth rule.", typ.Name, err)
	}

	idx := rbacRegex.FindAllStringSubmatchIndex(rule, -1)
	if len(idx) != 1 || len(idx[0]) != 8 || rule != rule[idx[0][0]:idx[0][1]] {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: `%s` is not a valid auth rule.", typ.Name, rule)
	}

	query := RBACQuery{
		Variable: rule[idx[0][2]:idx[0][3]],
		Operator: rule[idx[0][4]:idx[0][5]],
		Operand:  rule[idx[0][6]:idx[0][7]],
	}

	if !strings.HasPrefix(query.Variable, "$") {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: `%s` is not a valid GraphQL variable.", typ.Name, query.Variable)
	}
	query.Variable = query.Variable[1:]

	if query.Operator != "eq" {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: `%s` operator is not supported in this auth rule.", typ.Name, query.Operator)
	}
	return &query, nil
}

func gqlValidateRule(
	s *ast.Schema,
	typ *ast.Definition,
	rule string,
	position *ast.Position,
	dgraphPredicate map[string]map[string]string,
	typeNameAst map[string][]*ast.Definition) (*query, error) {

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: rule})
	if gqlErr != nil {
		return nil, x.GqlErrorf(
			"failed to parse GraphQL rule [reason : %s]", toGqlError(gqlErr).Error()).
			WithLocations(x.Location{Line: position.Line, Column: position.Column})
	}

	if len(doc.Operations) != 1 {
		return nil, gqlerror.ErrorPosf(position,
			"a rule should be exactly one query, found %v GraphQL operations", len(doc.Operations))
	}
	op := doc.Operations[0]

	if op == nil {
		return nil, gqlerror.ErrorPosf(position,
			"a rule should be exactly one query, found an empty GraphQL operation")
	}

	if op.Operation != "query" {
		return nil, gqlerror.ErrorPosf(position,
			"a rule should be exactly one query, found an %s", op.Name)
	}

	listErr := validator.Validate(s, doc)
	if len(listErr) != 0 {
		var errs error
		for _, err := range listErr {
			errs = AppendGQLErrs(
				errs,
				x.GqlErrorf("failed to validate GraphQL rule [reason : %s]", toGqlError(err)).
					WithLocations(x.Location{Line: position.Line, Column: position.Column}))
		}
		return nil, errs
	}

	if len(op.SelectionSet) != 1 {
		return nil, gqlerror.ErrorPosf(position,
			"a rule should be exactly one query, found %v queries", len(op.SelectionSet))
	}

	f, ok := op.SelectionSet[0].(*ast.Field)
	if !ok {
		return nil, gqlerror.ErrorPosf(position,
			"error couldn't generate query from rule")
	}

	if f.Name != "query"+typ.Name {
		return nil, gqlerror.ErrorPosf(position,
			"on type %s expected only query%s rules,but found %s", typ.Name, typ.Name, f.Name)
	}

	return &query{
		field: f,
		op: &operation{op: op,
			query: rule,
			doc:   doc,
			inSchema: &schema{schema: s,
				dgraphPredicate: dgraphPredicate,
				typeNameAst:     typeNameAst,
			},
			// need to fill in vars at query time
		},
		sel: op.SelectionSet[0]}, nil
}
