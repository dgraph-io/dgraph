/*
 * Copyright 2015 Manish R Jain <manishrjain@gmaicom>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gql

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
)

// GraphQuery stores the parsed Query in a tree format. This gets converted to
// internally used query.SubGraph before processing the query.
type GraphQuery struct {
	UID      uint64
	XID      string
	Attr     string
	First    int
	Offset   int
	After    uint64
	Children []*GraphQuery

	// Internal fields below.
	// If gq.fragment is nonempty, then it is a fragment reference / spread.
	fragment string
}

// Mutation stores the strings corresponding to set and delete operations.
type Mutation struct {
	Set string
	Del string
}

// pair denotes the key value pair that is part of the GraphQL query root in parenthesis.
type pair struct {
	Key string
	Val string
}

// Internal structure for doing dfs on fragments.
type fragmentNode struct {
	Name    string
	Gq      *GraphQuery
	Entered bool // Entered in dfs.
	Exited  bool // Exited in dfs.
}

// Key is fragment names.
type fragmentMap map[string]*fragmentNode

// VarMap is a string map with key as variable name.
type VarMap map[string]string

// run is used to run the lexer until we encounter nil state.
func run(l *lex.Lexer) {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.Items) // No more tokens.
}

func (gq *GraphQuery) isFragment() bool {
	return gq.fragment != ""
}

func (fn *fragmentNode) expand(fmap fragmentMap) error {
	if fn.Exited {
		// This fragment node has already been expanded.
		return nil
	}
	if fn.Entered {
		return fmt.Errorf("Cycle detected: %s", fn.Name)
	}
	fn.Entered = true
	if err := fn.Gq.expandFragments(fmap); err != nil {
		return err
	}
	fn.Exited = true
	return nil
}

func (gq *GraphQuery) expandFragments(fmap fragmentMap) error {
	// We have to make a copy of children to preserve order and replace
	// fragment references with fragment content. The copy is newChildren.
	var newChildren []*GraphQuery
	// Expand non-fragments. Do not append to gq.Children.
	for _, child := range gq.Children {
		if child.isFragment() {
			fname := child.fragment // Name of fragment being referenced.
			fchild := fmap[fname]
			if fchild == nil {
				return fmt.Errorf("Missing fragment: %s", fname)
			}
			if err := fchild.expand(fmap); err != nil {
				return err
			}
			newChildren = append(newChildren, fchild.Gq.Children...)
		} else {
			if err := child.expandFragments(fmap); err != nil {
				return err
			}
			newChildren = append(newChildren, child)
		}
	}
	gq.Children = newChildren
	return nil
}

type jsonQuery struct {
	Variables map[string]string `json:"variables"`
	Query     string            `json:"query"`
}

func checkForVariableList(str string) (string, VarMap, error) {
	var mp jsonQuery
	tempMap := make(VarMap)
	err := json.Unmarshal([]byte(str), &mp)
	if err != nil {
		return str, tempMap, nil // It does not obey GraphiQL format but valid
	}

	return mp.Query, mp.Variables, nil
}

func checkValidityOfVariables(mp1, mp2 VarMap) error {
	if len(mp1) != len(mp2) {
		return fmt.Errorf("Variable list incomplete. lengths are %v and %v",
			len(mp1), len(mp2))
	}

	for k := range mp1 {
		if _, ok := mp2[k]; !ok {
			return fmt.Errorf("Variable list incomplete. Missing: %v", k)
		}
	}

	for k := range mp2 {
		if _, ok := mp1[k]; !ok {
			return fmt.Errorf("Variable list incomplete. Missing: %v", k)
		}
	}
	return nil
}

func areAllVarsUsed(mp1 VarMap, mp2 map[string]bool) error {
	for k := range mp1 {
		if _, ok := mp2[k]; !ok {
			return fmt.Errorf("Variable %s declared and not used", k)
		}
	}

	return nil
}

// Parse initializes and runs the lexer. It also constructs the GraphQuery subgraph
// from the lexed items.
func Parse(input string) (gq *GraphQuery, mu *Mutation, rerr error) {
	l := &lex.Lexer{}
	query, varValuesMap, err := checkForVariableList(input)

	if err != nil {
		return nil, nil, err
	}

	l.Init(query)
	go run(l)

	fmap := make(fragmentMap)
	varMap := make(VarMap)
	varUsed := make(map[string]bool)
	for item := range l.Items {
		switch item.Typ {
		case itemText:
			continue

		case itemOpType:
			if item.Val == "mutation" {
				if mu != nil {
					return nil, nil, errors.New("Only one mutation block allowed.")
				}
				if mu, rerr = getMutation(l); rerr != nil {
					return nil, nil, rerr
				}
			} else if item.Val == "fragment" {
				// TODO(jchiu0): This is to be done in ParseSchema once it is ready.
				fnode, rerr := getFragment(l, varValuesMap, varUsed)
				if rerr != nil {
					return nil, nil, rerr
				}
				fmap[fnode.Name] = fnode
			}

		case itemLeftCurl:
			if gq == nil {
				if gq, rerr = getRoot(l, varValuesMap, varUsed); rerr != nil {
					return nil, nil, rerr
				}
			} else {
				if err := godeep(l, gq, varValuesMap, varUsed); err != nil {
					return nil, nil, err
				}
			}

		case itemLeftRound:
			parseVariables(l, varMap, varValuesMap)
		}
	}

	if err := checkValidityOfVariables(varMap, varValuesMap); err != nil {
		return nil, nil, err
	}
	if gq != nil {
		// Try expanding fragments using fragment map.
		if err := gq.expandFragments(fmap); err != nil {
			return nil, nil, err
		}
	}

	if err := areAllVarsUsed(varMap, varUsed); err != nil {
		return nil, nil, err
	}

	return gq, mu, nil
}

// getFragment parses a fragment definition (not reference).
func getFragment(l *lex.Lexer, varValuesMap VarMap, varUsed map[string]bool) (*fragmentNode, error) {
	var name string
	for item := range l.Items {
		if item.Typ == itemText {
			v := strings.TrimSpace(item.Val)
			if len(v) > 0 && name == "" {
				// Currently, we take the first nontrivial token as the
				// fragment name and ignore everything after that until we see
				// a left curl.
				name = v
			}
		} else if item.Typ == itemLeftCurl {
			break
		} else {
			return nil, fmt.Errorf("Unexpected item in fragment: %v %v",
				item.Typ, item.Val)
		}
	}
	if name == "" {
		return nil, errors.New("Empty fragment name")
	}
	gq := new(GraphQuery)
	if err := godeep(l, gq, varValuesMap, varUsed); err != nil {
		return nil, err
	}
	fn := &fragmentNode{
		Name: name,
		Gq:   gq,
	}
	return fn, nil
}

// getMutation function parses and stores the set and delete operation in Mutation.
func getMutation(l *lex.Lexer) (mu *Mutation, rerr error) {
	for item := range l.Items {
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			mu = new(Mutation)
		}
		if item.Typ == itemRightCurl {
			return mu, nil
		}
		if item.Typ == itemMutationOp {
			if err := parseMutationOp(l, item.Val, mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, errors.New("Invalid mutation.")
}

// parseMutationOp parses and stores set or delete operation string in Mutation.
func parseMutationOp(l *lex.Lexer, op string, mu *Mutation) error {
	if mu == nil {
		return errors.New("Mutation is nil.")
	}

	parse := false
	for item := range l.Items {
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			if parse {
				return errors.New("Too many left curls in set mutation.")
			}
			parse = true
		}
		if item.Typ == itemMutationContent {
			if !parse {
				return errors.New("Mutation syntax invalid.")
			}
			if op == "set" {
				mu.Set = item.Val
			} else if op == "delete" {
				mu.Del = item.Val
			} else {
				return errors.New("Invalid mutation operation.")
			}
		}
		if item.Typ == itemRightCurl {
			return nil
		}
	}
	return errors.New("Invalid mutation formatting.")
}

func parseVariables(l *lex.Lexer, varMap, varValuesMap VarMap) error {
	for {
		var varName string
		// Get variable name
		item := <-l.Items
		if item.Typ == itemVarName {
			varName = item.Val
		} else if item.Typ == itemRightRound {
			break
		} else {
			return fmt.Errorf("Expecting a variable name. Got: %v", item)
		}

		// Get variable type
		item = <-l.Items
		if item.Typ != itemVarType {
			return fmt.Errorf("Expecting a variable type. Got: %v", item)
		}

		varMap[varName] = item.Val
		item = <-l.Items
		if item.Typ == itemEqual {
			it := <-l.Items
			if it.Typ != itemVarDefault {
				return fmt.Errorf("Expecting default value of a variable. Got: %v", item)
			}

			if _, ok := varValuesMap[varName]; !ok {
				varValuesMap[varName] = it.Val
			}
			item = <-l.Items
		}

		if item.Typ == itemComma {
			continue
		} else if item.Typ == itemRightRound {
			break
		}

	}
	return nil
}

// parseArguments parses the arguments part of the GraphQL query root.
func parseArguments(l *lex.Lexer) (result []pair, rerr error) {
	for {
		var p pair

		// Get key
		item := <-l.Items
		if item.Typ == itemArgName {
			p.Key = item.Val

		} else if item.Typ == itemRightRound {
			break

		} else {
			return result, fmt.Errorf("Expecting argument name. Got: %v", item)
		}

		// Get value
		item = <-l.Items
		if item.Typ != itemArgVal {
			return result, fmt.Errorf("Expecting argument value. Got: %v", item)
		}

		p.Val = item.Val
		result = append(result, p)
	}
	return result, nil
}

// getRoot gets the root graph query object after parsing the args.
func getRoot(l *lex.Lexer, varValuesMap VarMap, varUsed map[string]bool) (gq *GraphQuery, rerr error) {
	gq = new(GraphQuery)
	item := <-l.Items
	if item.Typ != itemName {
		return nil, fmt.Errorf("Expected some name. Got: %v", item)
	}

	gq.Attr = item.Val
	item = <-l.Items
	if item.Typ != itemLeftRound {
		return nil, fmt.Errorf("Expected variable start. Got: %v", item)
	}

	args, err := parseArguments(l)
	if err != nil {
		return nil, err
	}
	for _, p := range args {
		if p.Key == "_uid_" {
			gq.UID, rerr = strconv.ParseUint(p.Val, 0, 64)
			if rerr != nil {
				return nil, rerr
			}
		} else if p.Key == "_xid_" {
			gq.XID = p.Val
		} else {
			return nil, fmt.Errorf("Expecting _uid_ or _xid_. Got: %+v", p)
		}
	}

	return gq, nil
}

// godeep constructs the subgraph from the lexed items and a GraphQuery node.
func godeep(l *lex.Lexer, gq *GraphQuery, varValuesMap VarMap, varUsed map[string]bool) error {
	curp := gq // Used to track current node, for nesting.
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			return errors.New(item.Val)
		}

		if item.Typ == lex.ItemEOF {
			return nil
		}

		if item.Typ == itemRightCurl {
			return nil
		}

		if item.Typ == itemFragmentSpread {
			// item.Val is expected to start with "..." and to have len >3.
			if len(item.Val) <= 3 {
				return fmt.Errorf("Fragment name invalid: %s", item.Val)
			}

			gq.Children = append(gq.Children, &GraphQuery{fragment: item.Val[3:]})
			// Unlike itemName, there is no nesting, so do not change "curp".

		} else if item.Typ == itemName {
			child := new(GraphQuery)
			child.Attr = item.Val
			gq.Children = append(gq.Children, child)
			curp = child

		} else if item.Typ == itemLeftCurl {
			if err := godeep(l, curp, varValuesMap, varUsed); err != nil {
				return err
			}

		} else if item.Typ == itemLeftRound {
			args, err := parseArguments(l)
			if err != nil {
				return err
			}
			// Stores args in GraphQuery, will be used later while retrieving results.
			for _, p := range args {
				// if the p.val is a variable(Starts with a $), Replace with the value.
				val := p.Val
				if p.Val[0] == '$' {
					val = varValuesMap[p.Val]
					varUsed[p.Val] = true
				}
				if p.Key == "first" {
					count, err := strconv.ParseInt(val, 0, 32)
					if err != nil {
						return err
					}
					curp.First = int(count)
				}
				if p.Key == "offset" {
					count, err := strconv.ParseInt(val, 0, 32)
					if err != nil {
						return err
					}
					if count < 0 {
						return errors.New("offset cannot be less than 0")
					}
					curp.Offset = int(count)
				}
				if p.Key == "after" {
					afterUid, err := strconv.ParseUint(val, 0, 64)
					if err != nil {
						return err
					}
					curp.After = afterUid
				}
			}
		}
	}
	return nil
}
