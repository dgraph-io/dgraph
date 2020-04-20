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
	"github.com/dgraph-io/dgraph/x"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
	"regexp"
	"strings"
)

const (
	RBACQueryPrefix = "{"
)

type RBACQuery struct {
	Variable string
	Operator string
	Operand  string
}

type RuleNode struct {
	Or       []*RuleNode
	And      []*RuleNode
	Not      *RuleNode
	Rule     Query
	RBACRule *RBACQuery
}

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

func authRules(s *ast.Schema) (map[string]*TypeAuth, error) {
	var errResult, err error
	authRules := make(map[string]*TypeAuth)

	for _, typ := range s.Types {
		name := typeName(typ)
		authRules[name] = &TypeAuth{fields: make(map[string]*AuthContainer)}
		auth := typ.Directives.ForName(authDirective)
		if auth != nil {
			authRules[name].rules, err = parseAuthDirective(s, typ, auth)
			errResult = AppendGQLErrs(errResult, err)
		}

		for _, field := range typ.Fields {
			auth := field.Directives.ForName(authDirective)
			if auth != nil {
				authRules[name].fields[field.Name], err = parseAuthDirective(s, typ, auth)
				errResult = AppendGQLErrs(errResult, err)
			}
		}
	}

	return authRules, errResult
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
		return nil, gqlerror.ErrorPosf(val.Position, "Type %s: @auth query: no arguments - "+
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
			errResult = AppendGQLErrs(errResult, gqlerror.ErrorPosf(ors.Position,
				`Type %s: @auth query: 'OR' should contain at least two rules`,
				typ.Name))
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
			errResult = AppendGQLErrs(errResult, gqlerror.ErrorPosf(ands.Position,
				`Type %s: @auth query: 'AND' should contain at least two rules`,
				typ.Name))
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
			result.RBACRule, err = rbacValidateRule(typ, rule.Raw, rule.Position)
		} else {
			result.Rule, err = gqlValidateRule(s, typ, rule.Raw, rule.Position)
		}
		errResult = AppendGQLErrs(errResult, err)
		numChildren++
	}

	if numChildren != 1 || len(val.Children) > 1 {
		errResult = AppendGQLErrs(errResult, gqlerror.ErrorPosf(val.Position,
			`Type %s: @auth query: there should be only one of "and", "or", "not" and "rule"`,
			typ.Name))
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
	position *ast.Position) (Query, error) {

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: rule})
	if gqlErr != nil {
		return nil, x.GqlErrorf("Type %s: @auth query: failed to parse GraphQL rule [reason : %s]",
			typ.Name, toGqlError(gqlErr).Error()).WithLocations(x.Location{Line: position.Line,
			Column: position.Column})
	}

	if len(doc.Operations) != 1 {
		return nil, gqlerror.ErrorPosf(position, "Type %s: @auth query: a rule should be "+
			"exactly one query, found %v GraphQL operations", typ.Name, len(doc.Operations))
	}
	op := doc.Operations[0]

	if op == nil {
		return nil, gqlerror.ErrorPosf(position, "Type %s: @auth query: a rule should be "+
			"exactly one query, found an empty GraphQL operation", typ.Name)
	}

	if op.Operation != "query" {
		return nil, gqlerror.ErrorPosf(position, "Type %s: @auth query: a rule should be exactly"+
			" one query, found an %s", typ.Name, op.Name)
	}

	listErr := validator.Validate(s, doc)
	if len(listErr) != 0 {
		var errs error
		for _, err := range listErr {
			errs = AppendGQLErrs(errs,
				x.GqlErrorf("Type %s: @auth query: failed to validate GraphQL rule [reason : %s]",
					typ.Name, toGqlError(err)).WithLocations(x.Location{Line: position.Line,
					Column: position.Column}))
		}
		return nil, errs
	}

	if len(op.SelectionSet) != 1 {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: @auth query: a rule should be exactly one query, found %v queries",
			typ.Name, len(op.SelectionSet))
	}

	f, ok := op.SelectionSet[0].(*ast.Field)
	if !ok {
		return nil, gqlerror.ErrorPosf(position,
			"Type %s: @auth query: error couldn't generate query from rule", typ.Name)
	}

	if f.Name != "query"+typ.Name {
		return nil, gqlerror.ErrorPosf(position, "Type %s: @auth query: expected only query%s"+
			" rules,but found %s", typ.Name, typ.Name, f.Name)
	}

	return &query{
		field: f,
		op: &operation{op: op,
			query: rule,
			doc:   doc,
			// need to fill in vars and schema at query time
		},
		sel: op.SelectionSet[0]}, nil
}
