/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package gql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

// GraphQuery stores the parsed Query in a tree format. This gets converted to
// internally used query.SubGraph before processing the query.
type GraphQuery struct {
	UID        []uint64
	Attr       string
	Langs      []string
	Alias      string
	IsCount    bool
	IsInternal bool
	Var        string
	NeedsVar   []string
	Func       *Function

	Args         map[string]string
	Children     []*GraphQuery
	Filter       *FilterTree
	Normalize    bool
	Facets       *Facets
	FacetsFilter *FilterTree

	// Internal fields below.
	// If gq.fragment is nonempty, then it is a fragment reference / spread.
	fragment string
}

// Mutation stores the strings corresponding to set and delete operations.
type Mutation struct {
	Set    string
	Del    string
	Schema string
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

type varInfo struct {
	Value string
	Type  string
}

// varMap is a map with key as variable name.
type varMap map[string]varInfo

// FilterTree is the result of parsing the filter directive.
// Either you can have `Op and Children` on non-leaf nodes
// Or Func at leaf nodes.
type FilterTree struct {
	Op    string
	Child []*FilterTree
	Func  *Function
}

// Function holds the information about gql functions.
type Function struct {
	Attr     string
	Lang     string   // language of the attribute value
	Name     string   // Specifies the name of the function.
	Args     []string // Contains the arguments of the function.
	NeedsVar []string // If the function requires some variable
}

// Facet holds the information about gql Facets (edge key-value pairs).
type Facets struct {
	AllKeys bool
	Keys    []string // should be in sorted order.
}

// filterOpPrecedence is a map from filterOp (a string) to its precedence.
var filterOpPrecedence map[string]int

func init() {
	filterOpPrecedence = map[string]int{
		"not": 3,
		"and": 2,
		"or":  1,
	}
}

func (f *Function) IsAggregator() bool {
	return isAggregator(f.Name)
}

func (f *Function) IsPasswordVerifier() bool {
	return f.Name == "checkpwd"
}

// DebugPrint is useful for debugging.
func (gq *GraphQuery) DebugPrint(prefix string) {
	x.Printf("%s[%x %q %q]\n", prefix, gq.UID, gq.Attr, gq.Alias)
	for _, c := range gq.Children {
		c.DebugPrint(prefix + "|->")
	}
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
		return x.Errorf("Cycle detected: %s", fn.Name)
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
				return x.Errorf("Missing fragment: %s", fname)
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

type query struct {
	Variables map[string]string `json:"variables"`
	Query     string            `json:"query"`
}

type queryAlt struct {
	Variables string `json:"variables"`
	Query     string `json:"query"`
}

/*
The query could be of the following forms :

# Normal query.
	`query test($a: int) { me(_xid_: alice-in-wonderland) {author(first:$a){name}}}`

# With stringified variables map.
 `{
   "query": "query test($a: int){ me(_xid_: alice-in-wonderland) {author(first:$a){name}}}",
	 "variables": "{'$a':'2'}"
	 }`

# With a non-stringified variables map.
  `{
   "query": "query test($a: int){ me(_xid_: alice-in-wonderland) {author(first:$a){name}}}",
	 "variables": {'$a':'2'}
	 }`
*/

func parseQueryWithVariables(str string) (string, varMap, error) {
	var q query
	vm := make(varMap)
	mp := make(map[string]string)
	if err := json.Unmarshal([]byte(str), &q); err != nil {
		// Check if the json object is stringified.
		var q1 queryAlt
		if err := json.Unmarshal([]byte(str), &q1); err != nil {
			return str, vm, nil // It does not obey GraphiQL format but valid.
		}
		// Convert the stringified variables to map if it is not nil.
		if q1.Variables != "" {
			if err = json.Unmarshal([]byte(q1.Variables), &mp); err != nil {
				return "", nil, err
			}
		}
	} else {
		mp = q.Variables
	}

	for k, v := range mp {
		vm[k] = varInfo{
			Value: v,
		}
	}
	return q.Query, vm, nil
}

func checkValueType(vm varMap) error {
	for k, v := range vm {
		typ := v.Type

		if len(typ) == 0 {
			return x.Errorf("Type of variable %v not specified", k)
		}

		// Ensure value is not nil if the variable is required.
		if typ[len(typ)-1] == '!' {
			if v.Value == "" {
				return x.Errorf("Variable %v should be initialised", k)
			}
			typ = typ[:len(typ)-1]
		}

		// Type check the values.
		if v.Value != "" {
			switch typ {
			case "int":
				{
					if _, err := strconv.ParseInt(v.Value, 0, 64); err != nil {
						return x.Wrapf(err, "Expected an int but got %v", v.Value)
					}
				}
			case "float":
				{
					if _, err := strconv.ParseFloat(v.Value, 64); err != nil {
						return x.Wrapf(err, "Expected a float but got %v", v.Value)
					}
				}
			case "bool":
				{
					if _, err := strconv.ParseBool(v.Value); err != nil {
						return x.Wrapf(err, "Expected a bool but got %v", v.Value)
					}
				}
			case "string": // Value is a valid string. No checks required.
			default:
				return x.Errorf("Type %v not supported", typ)
			}
		}
	}

	return nil
}

func substituteVariables(gq *GraphQuery, vmap varMap) error {
	for k, v := range gq.Args {
		// v won't be empty as its handled in parseVariables.
		if v[0] == '$' {
			va, ok := vmap[v]
			if !ok {
				return x.Errorf("Variable not defined %v", v)
			}
			gq.Args[k] = va.Value
		}
	}

	for _, child := range gq.Children {
		if err := substituteVariables(child, vmap); err != nil {
			return err
		}
	}

	return nil
}

// Vars struct contains the list of variables defined and used by a
// query block.
type Vars struct {
	Defines []string
	Needs   []string
}

// Result struct contains the Query list, its corresponding variable use list
// and the mutation block.
type Result struct {
	Query     []*GraphQuery
	QueryVars []*Vars
	Mutation  *Mutation
	Schema    *graphp.Schema
}

// Parse initializes and runs the lexer. It also constructs the GraphQuery subgraph
// from the lexed items.
func Parse(input string) (res Result, rerr error) {
	query, vmap, err := parseQueryWithVariables(input)
	if err != nil {
		return res, err
	}

	l := lex.NewLexer(query).Run(lexText)

	var qu *GraphQuery
	it := l.NewIterator()
	fmap := make(fragmentMap)
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return res, x.Errorf(item.Val)
		case itemOpType:
			if item.Val == "mutation" {
				if res.Mutation != nil {
					return res, x.Errorf("Only one mutation block allowed.")
				}
				if res.Mutation, rerr = getMutation(it); rerr != nil {
					return res, rerr
				}
			} else if item.Val == "schema" {
				if res.Schema != nil {
					return res, x.Errorf("Only one schema block allowed ")
				}
				if res.Query != nil {
					return res, x.Errorf("schema block is not allowed with query block")
				}
				if res.Schema, rerr = getSchema(it); rerr != nil {
					return res, rerr
				}
			} else if item.Val == "fragment" {
				// TODO(jchiu0): This is to be done in ParseSchema once it is ready.
				fnode, rerr := getFragment(it)
				if rerr != nil {
					return res, rerr
				}
				fmap[fnode.Name] = fnode
			} else if item.Val == "query" {
				if res.Schema != nil {
					return res, x.Errorf("schema block is not allowed with query block")
				}
				if qu, rerr = getVariablesAndQuery(it, vmap); rerr != nil {
					return res, rerr
				}
				res.Query = append(res.Query, qu)
			}
		case itemLeftCurl:
			if qu, rerr = getQuery(it); rerr != nil {
				return res, rerr
			}
			res.Query = append(res.Query, qu)
		case itemName:
			it.Prev()
			if qu, rerr = getQuery(it); rerr != nil {
				return res, rerr
			}
			res.Query = append(res.Query, qu)
		}
	}

	if len(res.Query) != 0 {
		res.QueryVars = make([]*Vars, 0, len(res.Query))
		for i := 0; i < len(res.Query); i++ {
			qu := res.Query[i]
			// Try expanding fragments using fragment map.
			if err := qu.expandFragments(fmap); err != nil {
				return res, err
			}

			// Substitute all variables with corresponding values
			if err := substituteVariables(qu, vmap); err != nil {
				return res, err
			}

			res.QueryVars = append(res.QueryVars, &Vars{})
			// Collect vars used and defined in Result struct.
			qu.collectVars(res.QueryVars[i])
		}
		if err := checkDependency(res.QueryVars); err != nil {
			return res, err
		}
	}
	return res, nil
}

func checkDependency(vl []*Vars) error {
	needs, defines := make([]string, 0, 10), make([]string, 0, 10)
	for _, it := range vl {
		needs = append(needs, it.Needs...)
		defines = append(defines, it.Defines...)
	}

	sort.Strings(needs)
	sort.Strings(defines)

	out := defines[:0]
	dlen := len(defines)
	if dlen > 0 {
		for i := 1; i < dlen; i++ {
			if defines[i-1] == defines[i] {
				continue
			}
			out = append(out, defines[i-1])
		}
		out = append(out, defines[dlen-1])
		defines = out
		if len(defines) != dlen {
			return x.Errorf("Some variables are declared multiple times.")
		}
	}

	out = needs[:0]
	nlen := len(needs)
	if nlen > 0 {
		for i := 1; i < nlen; i++ {
			if needs[i-1] == needs[i] {
				continue
			}
			out = append(out, needs[i-1])
		}
		out = append(out, needs[nlen-1])
		needs = out
	}
	if len(defines) > len(needs) {
		return x.Errorf("Some variables are defined but not used\nDefined:%v\nUsed:%v\n",
			defines, needs)
	}

	if len(defines) < len(needs) {
		return x.Errorf("Some variables are used but not defined\nDefined:%v\nUsed:%v\n",
			defines, needs)
	}

	return nil
}

func (qu *GraphQuery) collectVars(v *Vars) {
	if qu.Var != "" {
		v.Defines = append(v.Defines, qu.Var)
	}

	v.Needs = append(v.Needs, qu.NeedsVar...)

	for _, ch := range qu.Children {
		ch.collectVars(v)
	}
	if qu.Filter != nil {
		qu.Filter.collectVars(v)
	}
}

func (f *FilterTree) collectVars(v *Vars) {
	if f.Func != nil {
		v.Needs = append(v.Needs, f.Func.NeedsVar...)
	}
	for _, fch := range f.Child {
		fch.collectVars(v)
	}
}

func (f *FilterTree) hasVars() bool {
	if (f.Func != nil) && (len(f.Func.NeedsVar) > 0) {
		return true
	}
	for _, fch := range f.Child {
		if fch.hasVars() {
			return true
		}
	}
	return false
}

// getVariablesAndQuery checks if the query has a variable list and stores it in
// vmap. For variable list to be present, the query should have a name which is
// also checked for. It also calls getQuery to create the GraphQuery object tree.
func getVariablesAndQuery(it *lex.ItemIterator, vmap varMap) (gq *GraphQuery,
	rerr error) {
	var name string
L2:
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return nil, x.Errorf(item.Val)
		case itemText:
			v := strings.TrimSpace(item.Val)
			if len(v) > 0 {
				if name != "" {
					return nil, x.Errorf("Multiple word query name not allowed.")
				}
				name = item.Val
			}
		case itemLeftRound:
			if name == "" {
				return nil, x.Errorf("Variables can be defined only in named queries.")
			}

			if rerr = parseVariables(it, vmap); rerr != nil {
				return nil, rerr
			}

			if rerr = checkValueType(vmap); rerr != nil {
				return nil, rerr
			}
		case itemLeftCurl:
			if gq, rerr = getQuery(it); rerr != nil {
				return nil, rerr
			}
			break L2
		}
	}
	return gq, nil
}

// getQuery creates a GraphQuery object tree by calling getRoot
// and goDeep functions by looking at '{'.
func getQuery(it *lex.ItemIterator) (gq *GraphQuery, rerr error) {
	// First, get the root
	gq, rerr = getRoot(it)
	if rerr != nil {
		return nil, rerr
	}

	var seenFilter bool
L:
	// Recurse to deeper levels through godeep.
	if !it.Next() {
		return nil, x.Errorf("Invalid query")
	}

	item := it.Item()
	if item.Typ == itemLeftCurl {
		if rerr = godeep(it, gq); rerr != nil {
			return nil, rerr
		}
	} else if item.Typ == itemAt {
		it.Next()
		item := it.Item()
		if item.Typ == itemName {
			switch item.Val {
			case "filter":
				if seenFilter {
					return nil, x.Errorf("Repeated filter at root")
				}
				seenFilter = true
				filter, err := parseFilter(it)
				if err != nil {
					return nil, err
				}
				gq.Filter = filter

			case "normalize":
				gq.Normalize = true
			default:
				return nil, x.Errorf("Unknown directive [%s]", item.Val)
			}
			goto L
		}
	} else {
		return nil, x.Errorf("Malformed Query. Missing {")
	}

	return gq, nil
}

// getFragment parses a fragment definition (not reference).
func getFragment(it *lex.ItemIterator) (*fragmentNode, error) {
	var name string
	for it.Next() {
		item := it.Item()
		if item.Typ == itemName {
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
			return nil, x.Errorf("Unexpected item in fragment: %v %v", item.Typ, item.Val)
		}
	}
	if name == "" {
		return nil, x.Errorf("Empty fragment name")
	}

	gq := &GraphQuery{
		Args: make(map[string]string),
	}
	if err := godeep(it, gq); err != nil {
		return nil, err
	}
	fn := &fragmentNode{
		Name: name,
		Gq:   gq,
	}
	return fn, nil
}

// getMutation function parses and stores the set and delete
// operation in Mutation.
func getMutation(it *lex.ItemIterator) (*Mutation, error) {
	var mu *Mutation
	for it.Next() {
		item := it.Item()
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
			if err := parseMutationOp(it, item.Val, mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, x.Errorf("Invalid mutation.")
}

// parses till rightSquare is found (parses [a, b]) excluding leftSquare
// This function can be reused for query later
func parseListItemNames(it *lex.ItemIterator) ([]string, error) {
	var items []string
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemRightSquare:
			return items, nil
		case itemName:
			items = append(items, item.Val)
		case itemComma:
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return items, x.Errorf("Invalid scheam block")
			}
			items = append(items, item.Val)
		default:
			return items, x.Errorf("Invalid schema block")
		}
	}
	return items, x.Errorf("Invalid schema block")
}

// parses till rightround is found
func parseSchemaPredicates(it *lex.ItemIterator, s *graphp.Schema) error {
	// pred should be followed by colon
	it.Next()
	item := it.Item()
	if item.Typ != itemName && item.Val != "pred" {
		return x.Errorf("Invalid schema block")
	}
	it.Next()
	item = it.Item()
	if item.Typ != itemColon {
		return x.Errorf("Invalid schema block")
	}

	// can be a or [a,b]
	it.Next()
	item = it.Item()
	if item.Typ == itemName {
		s.Predicates = append(s.Predicates, item.Val)
	} else if item.Typ == itemLeftSquare {
		var err error
		if s.Predicates, err = parseListItemNames(it); err != nil {
			return err
		}
	} else {
		return x.Errorf("Invalid schema block")
	}

	it.Next()
	item = it.Item()
	if item.Typ == itemRightRound {
		return nil
	}
	return x.Errorf("Invalid schema blocks")
}

// parses till rightcurl is found
func parseSchemaFields(it *lex.ItemIterator, s *graphp.Schema) error {
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemRightCurl:
			return nil
		case itemName:
			s.Fields = append(s.Fields, item.Val)
		default:
			return x.Errorf("Invalid schema block.")
		}
	}
	return x.Errorf("Invalid schema block.")
}

func getSchema(it *lex.ItemIterator) (*graphp.Schema, error) {
	var s graphp.Schema
	leftRoundSeen := false
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemLeftCurl:
			if err := parseSchemaFields(it, &s); err != nil {
				return nil, err
			}
			return &s, nil
		case itemLeftRound:
			if leftRoundSeen {
				return nil, x.Errorf("Too many left rounds in schema block")
			}
			leftRoundSeen = true
			if err := parseSchemaPredicates(it, &s); err != nil {
				return nil, err
			}
		default:
			return nil, x.Errorf("Invalid schema block")
		}
	}
	return nil, x.Errorf("Invalid schema block.")
}

// parseMutationOp parses and stores set or delete operation string in Mutation.
func parseMutationOp(it *lex.ItemIterator, op string, mu *Mutation) error {
	if mu == nil {
		return x.Errorf("Mutation is nil.")
	}

	parse := false
	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			if parse {
				return x.Errorf("Too many left curls in set mutation.")
			}
			parse = true
		}
		if item.Typ == itemMutationContent {
			if !parse {
				return x.Errorf("Mutation syntax invalid.")
			}
			if op == "set" {
				mu.Set = item.Val
			} else if op == "delete" {
				mu.Del = item.Val
			} else if op == "schema" {
				mu.Schema = item.Val
			} else {
				return x.Errorf("Invalid mutation operation.")
			}
		}
		if item.Typ == itemRightCurl {
			return nil
		}
	}
	return x.Errorf("Invalid mutation formatting.")
}

func parseVariables(it *lex.ItemIterator, vmap varMap) error {
	expectArg := true
	for it.Next() {
		var varName string
		// Get variable name.
		item := it.Item()
		if item.Typ == itemDollar {
			if !expectArg {
				return x.Errorf("Missing comma in var declaration")
			}
			it.Next()
			item = it.Item()
			if item.Typ == itemName {
				varName = fmt.Sprintf("$%s", item.Val)
			} else {
				return x.Errorf("Expecting a variable name. Got: %v", item)
			}
		} else if item.Typ == itemRightRound {
			if expectArg {
				return x.Errorf("Invalid comma in var block")
			}
			break
		} else if item.Typ == itemComma {
			if expectArg {
				return x.Errorf("Invalid comma in var block")
			}
			expectArg = true
			continue
		} else {
			return x.Errorf("Unexpected item in place of variable. Got: %v %v", item, item.Typ == itemDollar)
		}

		it.Next()
		item = it.Item()
		if item.Typ != itemColon {
			return x.Errorf("Expecting a collon. Got: %v", item)
		}

		// Get variable type.
		it.Next()
		item = it.Item()
		if item.Typ != itemName {
			return x.Errorf("Expecting a variable type. Got: %v", item)
		}

		// Ensure that the type is not nil.
		varType := item.Val
		if varType == "" {
			return x.Errorf("Type of a variable can't be empty")
		}

		// Insert the variable into the map. The variable might already be defiend
		// in the variable list passed with the query.
		if _, ok := vmap[varName]; ok {
			vmap[varName] = varInfo{
				Value: vmap[varName].Value,
				Type:  varType,
			}
		} else {
			vmap[varName] = varInfo{
				Type: varType,
			}
		}

		// Check for '=' sign and optional default value.
		it.Next()
		item = it.Item()
		if item.Typ == itemEqual {
			it.Next()
			it := it.Item()
			if it.Typ != itemName {
				return x.Errorf("Expecting default value of a variable. Got: %v", item)
			}

			if varType[len(varType)-1] == '!' {
				return x.Errorf("Type ending with ! can't have default value: Got: %v", varType)
			}

			// If value is empty replace, otherwise ignore the default value
			// as the intialised value will override the default value.
			if vmap[varName].Value == "" {
				vmap[varName] = varInfo{
					Value: it.Val,
					Type:  varType,
				}
			}
		} else if item.Typ == itemRightRound {
			break
		} else {
			// We consumed an extra item to see if it was an '=' sign, so move back.
			it.Prev()
		}
		expectArg = false
	}
	return nil
}

// parseArguments parses the arguments part of the GraphQL query root.
func parseArguments(it *lex.ItemIterator, gq *GraphQuery) (result []pair, rerr error) {
	expectArg := true
	for it.Next() {
		var p pair
		// Get key.
		item := it.Item()
		if item.Typ == itemName {
			if !expectArg {
				return result, x.Errorf("Expecting a comma. But got: %v", item.Val)
			}
			p.Key = item.Val
			expectArg = false
		} else if item.Typ == itemRightRound {
			if expectArg {
				return result, x.Errorf("Unexpected comma before ).")
			}
			break
		} else if item.Typ == itemComma {
			if expectArg {
				return result, x.Errorf("Expected Argument but got comma.")
			}
			expectArg = true
			continue
		} else {
			return result, x.Errorf("Expecting argument name. Got: %v", item)
		}

		it.Next()
		item = it.Item()
		if item.Typ != itemColon {
			return result, x.Errorf("Expecting a collon. Got: %v in %v", item, gq.Attr)
		}

		// Get value.
		it.Next()
		item = it.Item()
		var val string
		if item.Val == "var" {
			count, err := parseVarList(it, gq)
			if err != nil {
				return result, err
			}
			if count != 1 {
				return result, x.Errorf("Only one variable expected. Got %d", count)
			}
			p.Val = gq.NeedsVar[len(gq.NeedsVar)-1]
			result = append(result, p)
			continue
		}

		if item.Typ == itemDollar {
			val = "$"
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return result, x.Errorf("Expecting argument value. Got: %v", item)
			}
		} else if item.Typ != itemName {
			return result, x.Errorf("Expecting argument value. Got: %v", item)
		}

		p.Val = val + item.Val
		result = append(result, p)
	}
	return result, nil
}

// debugString converts FilterTree to a string. Good for testing, debugging.
func (t *FilterTree) debugString() string {
	buf := bytes.NewBuffer(make([]byte, 0, 20))
	t.stringHelper(buf)
	return buf.String()
}

// stringHelper does simple DFS to convert FilterTree to string.
func (t *FilterTree) stringHelper(buf *bytes.Buffer) {
	x.AssertTrue(t != nil)
	if t.Func != nil && len(t.Func.Name) > 0 {
		// Leaf node.
		buf.WriteRune('(')
		buf.WriteString(t.Func.Name)

		if len(t.Func.Attr) > 0 {
			buf.WriteRune(' ')
			buf.WriteString(t.Func.Attr)
			if len(t.Func.Lang) > 0 {
				buf.WriteRune('@')
				buf.WriteString(t.Func.Lang)
			}

			for _, arg := range t.Func.Args {
				buf.WriteString(" \"")
				buf.WriteString(arg)
				buf.WriteRune('"')
			}
		}
		buf.WriteRune(')')
		return
	}
	// Non-leaf node.
	buf.WriteRune('(')
	switch t.Op {
	case "and":
		buf.WriteString("AND")
	case "or":
		buf.WriteString("OR")
	case "not":
		buf.WriteString("NOT")
	default:
		x.Fatalf("Unknown operator: %q", t.Op)
	}

	for _, c := range t.Child {
		buf.WriteRune(' ')
		c.stringHelper(buf)
	}
	buf.WriteRune(')')
}

type filterTreeStack struct{ a []*FilterTree }

func (s *filterTreeStack) empty() bool        { return len(s.a) == 0 }
func (s *filterTreeStack) size() int          { return len(s.a) }
func (s *filterTreeStack) push(t *FilterTree) { s.a = append(s.a, t) }

func (s *filterTreeStack) pop() (*FilterTree, error) {
	if s.empty() {
		return nil, x.Errorf("Empty stack")
	}
	last := s.a[len(s.a)-1]
	s.a = s.a[:len(s.a)-1]
	return last, nil
}

func (s *filterTreeStack) peek() *FilterTree {
	x.AssertTruef(!s.empty(), "Trying to peek empty stack")
	return s.a[len(s.a)-1]
}

func evalStack(opStack, valueStack *filterTreeStack) error {
	topOp, err := opStack.pop()
	if err != nil {
		return x.Errorf("Invalid filter statement")
	}
	if topOp.Op == "not" {
		// Since "not" is a unary operator, just pop one value.
		topVal, err := valueStack.pop()
		if err != nil {
			return x.Errorf("Invalid filter statement")
		}
		topOp.Child = []*FilterTree{topVal}
	} else {
		// "and" and "or" are binary operators, so pop two values.
		if valueStack.size() < 2 {
			return x.Errorf("Invalid filter statement")
		}
		topVal1, _ := valueStack.pop()
		topVal2, _ := valueStack.pop()
		topOp.Child = []*FilterTree{topVal2, topVal1}
	}
	// Push the new value (tree) into the valueStack.
	valueStack.push(topOp)
	return nil
}

func parseFunction(it *lex.ItemIterator) (*Function, error) {
	var g *Function
	var expectArg, seenFuncArg, expectLang bool
L:
	for it.Next() {
		item := it.Item()
		if item.Typ == itemName { // Value.
			g = &Function{Name: strings.ToLower(item.Val)}
			it.Next()
			itemInFunc := it.Item()
			if itemInFunc.Typ != itemLeftRound {
				return nil, x.Errorf("Expected ( after func name [%s] but got %v",
					g.Name, itemInFunc.Val)
			}
			expectArg = true
			for it.Next() {
				itemInFunc := it.Item()
				if itemInFunc.Typ == itemRightRound {
					break L
				} else if itemInFunc.Typ == itemComma {
					expectArg = true
					continue
				} else if itemInFunc.Typ == itemLeftRound {
					// Function inside a function.
					if seenFuncArg {
						return nil, x.Errorf("Multiple functions as arguments not allowed")
					}
					it.Prev()
					it.Prev()
					f, err := parseFunction(it)
					if err != nil {
						return nil, err
					}
					seenFuncArg = true
					g.Attr = f.Attr
					g.Args = append(g.Args, f.Name)
					continue
				} else if itemInFunc.Typ == itemAt {
					if len(g.Attr) > 0 && len(g.Lang) == 0 {
						itNext, err := it.Peek(1)
						if err == nil && itNext[0].Val == "filter" {
							return nil, x.Errorf("Filter cannot be used inside a function.")
						}
						expectLang = true
						continue
					} else {
						return nil, x.Errorf("Invalid usage of '@' in function argument")
					}
				} else if itemInFunc.Typ != itemName {
					return nil, x.Errorf("Expected arg after func [%s], but got item %v",
						g.Name, itemInFunc)
				}
				if !expectArg && !expectLang {
					return nil, x.Errorf("Expected comma or language but got: %s", itemInFunc.Val)
				}
				val := strings.Trim(itemInFunc.Val, "\" \t")
				if val == "" {
					return nil, x.Errorf("Empty argument received")
				}
				if len(g.Attr) == 0 {
					if strings.ContainsRune(itemInFunc.Val, '"') {
						return nil, x.Errorf("Attribute in function must not be quoted with \": %s",
							itemInFunc.Val)
					}
					g.Attr = val
				} else if expectLang {
					g.Lang = val
					expectLang = false
				} else {
					g.Args = append(g.Args, val)
				}
				if g.Name == "var" {
					g.NeedsVar = append(g.NeedsVar, val)
				}
				expectArg = false
			}
		} else {
			return nil, x.Errorf("Expected a function but got %q", item.Val)
		}
	}
	return g, nil
}

func parseFacets(it *lex.ItemIterator) (*Facets, *FilterTree, error) {
	facets := new(Facets)
	peeks, err := it.Peek(1)
	expectArg := true
	if err == nil && peeks[0].Typ == itemLeftRound {
		it.Next() // ignore '('
		// parse comma separated strings (a1,b1,c1)
		done := false
		numTokens := 0
		for it.Next() {
			numTokens++
			item := it.Item()
			if item.Typ == itemRightRound { // done
				done = true
				break
			} else if item.Typ == itemName {
				if !expectArg {
					return nil, nil, x.Errorf("Expected a comma but got %v", item.Val)
				}
				facets.Keys = append(facets.Keys, item.Val)
				expectArg = false
			} else if item.Typ == itemComma {
				if expectArg {
					return nil, nil, x.Errorf("Expected Argument but got comma.")
				}
				expectArg = true
				continue
			} else {
				break
			}
		}
		if !done {
			// this is not (facet1, facet2, facet3)
			// try parsing filters. (eq(facet1, val1) AND eq(facet2, val2)...)
			// revert back tokens
			for i := 0; i < numTokens+1; i++ { // +1 for starting '('
				it.Prev()
			}
			filterTree, err := parseFilter(it)
			return nil, filterTree, err
		}
	}
	if len(facets.Keys) == 0 {
		facets.AllKeys = true
	} else {
		sort.Strings(facets.Keys)
		// deduplicate facets
		out := facets.Keys[:0]
		flen := len(facets.Keys)
		for i := 1; i < flen; i++ {
			if facets.Keys[i-1] == facets.Keys[i] {
				continue
			}
			out = append(out, facets.Keys[i-1])
		}
		out = append(out, facets.Keys[flen-1])
		facets.Keys = out
	}
	return facets, nil, nil
}

// parseFilter parses the filter directive to produce a QueryFilter / parse tree.
func parseFilter(it *lex.ItemIterator) (*FilterTree, error) {
	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected ( after filter directive")
	}

	// opStack is used to collect the operators in right order.
	opStack := new(filterTreeStack)
	opStack.push(&FilterTree{Op: "("}) // Push ( onto operator stack.
	// valueStack is used to collect the values.
	valueStack := new(filterTreeStack)

	for it.Next() {
		item := it.Item()
		lval := strings.ToLower(item.Val)
		if lval == "and" || lval == "or" || lval == "not" { // Handle operators.
			op := lval
			opPred := filterOpPrecedence[op]
			x.AssertTruef(opPred > 0, "Expected opPred > 0: %d", opPred)
			// Evaluate the stack until we see an operator with strictly lower pred.
			for !opStack.empty() {
				topOp := opStack.peek()
				if filterOpPrecedence[topOp.Op] < opPred {
					break
				}
				err := evalStack(opStack, valueStack)
				if err != nil {
					return nil, err
				}
			}
			opStack.push(&FilterTree{Op: op}) // Push current operator.
		} else if item.Typ == itemName { // Value.
			it.Prev()
			f, err := parseFunction(it)
			if err != nil {
				return nil, err
			}
			leaf := &FilterTree{Func: f}
			valueStack.push(leaf)
		} else if item.Typ == itemLeftRound { // Just push to op stack.
			opStack.push(&FilterTree{Op: "("})

		} else if item.Typ == itemRightRound { // Pop op stack until we see a (.
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Op == "(" {
					break
				}
				err := evalStack(opStack, valueStack)
				if err != nil {
					return nil, err
				}
			}
			_, err := opStack.pop() // Pop away the (.
			if err != nil {
				return nil, x.Errorf("Invalid filter statement")
			}
			if opStack.empty() {
				// The parentheses are balanced out. Let's break.
				break
			}
		} else {
			return nil, x.Errorf("Unexpected item while parsing @filter: %v", item)
		}
	}

	// For filters, we start with ( and end with ). We expect to break out of loop
	// when the parentheses balance off, and at that point, opStack should be empty.
	// For other applications, typically after all items are
	// consumed, we will run a loop like "while opStack is nonempty, evalStack".
	// This is not needed here.
	x.AssertTruef(opStack.empty(), "Op stack should be empty when we exit")

	if valueStack.empty() {
		// This happens when we have @filter(). We can either return an error or
		// ignore. Currently, let's just ignore and pretend there is no filter.
		return nil, nil
	}

	if valueStack.size() != 1 {
		return nil, x.Errorf("Expected one item in value stack, but got %d",
			valueStack.size())
	}
	return valueStack.pop()
}

func parseID(gq *GraphQuery, val string) error {
	val = x.WhiteSpace.Replace(val)
	toUid := func(str string) {
		uid, rerr := strconv.ParseUint(str, 0, 64)
		if rerr == nil {
			gq.UID = append(gq.UID, uid)
		} else {
			gq.UID = append(gq.UID, farm.Fingerprint64([]byte(str)))
		}
	}
	if val[0] != '[' {
		toUid(val)
		return nil
	}

	if val[len(val)-1] != ']' {
		return x.Errorf("Invalid id list at root. Got: %+v", val)
	}
	var buf bytes.Buffer
	for _, c := range val[1:] {
		if c == ',' || c == ']' {
			if buf.Len() == 0 {
				continue
			}
			toUid(buf.String())
			buf.Reset()
			continue
		}
		if c == '[' || c == ')' {
			return x.Errorf("Invalid id list at root. Got: %+v", val)
		}
		buf.WriteRune(c)
	}
	return nil
}

func parseVarList(it *lex.ItemIterator, gq *GraphQuery) (int, error) {
	count := 0
	expectArg := true
	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return count, x.Errorf("Expected a left round after var")
	}
	for it.Next() {
		item := it.Item()
		if item.Typ == itemRightRound {
			break
		}
		if item.Typ == itemComma {
			if expectArg {
				return count, x.Errorf("Expected a variable but got comma")
			}
			expectArg = true
		} else if item.Typ == itemName {
			if !expectArg {
				return count, x.Errorf("Expected a variable but got comma")
			}
			count++
			gq.NeedsVar = append(gq.NeedsVar, item.Val)
			expectArg = false
		}
	}
	if expectArg {
		return count, x.Errorf("Unnecessary comma in var()")
	}
	return count, nil
}

func parseDirective(it *lex.ItemIterator, curp *GraphQuery) error {
	it.Next()
	item := it.Item()
	peek, err := it.Peek(1)
	if err == nil && item.Typ == itemName {
		if item.Val == "facets" { // because @facets can come w/t '()'
			facets, facetsFilter, err := parseFacets(it)
			if err != nil {
				return err
			}
			if facets != nil {
				if curp.Facets != nil {
					return x.Errorf("Only one facets allowed")
				}
				curp.Facets = facets
			} else if facetsFilter != nil {
				if curp.FacetsFilter != nil {
					return x.Errorf("Only one facets filter allowed")
				}
				if facetsFilter.hasVars() {
					return x.Errorf(
						"variables are not allowed in facets filter.")
				}
				curp.FacetsFilter = facetsFilter
			} else {
				return x.Errorf("Facets parsing failed.")
			}
		} else if peek[0].Typ == itemLeftRound {
			// this is directive
			switch item.Val {
			case "filter":
				filter, err := parseFilter(it)
				if err != nil {
					return err
				}
				curp.Filter = filter

			default:
				return x.Errorf("Unknown directive [%s]", item.Val)
			}
		} else if len(curp.Attr) > 0 && len(curp.Langs) == 0 {
			// this is language list
			for ; item.Typ == itemName; item = it.Item() {
				curp.Langs = append(curp.Langs, item.Val)
				it.Next()
				if it.Item().Typ == itemColon {
					it.Next()
				} else {
					break
				}
			}
			it.Prev()
			if len(curp.Langs) == 0 {
				return x.Errorf("Expected at least 1 language in list for %s", curp.Attr)
			}
		} else {
			return x.Errorf("Expected directive or language list, got @%s", item.Val)
		}
	} else {
		return x.Errorf("Expected directive or language list")
	}
	return nil
}

// getRoot gets the root graph query object after parsing the args.
func getRoot(it *lex.ItemIterator) (gq *GraphQuery, rerr error) {
	gq = &GraphQuery{
		Args: make(map[string]string),
	}
	if !it.Next() {
		return nil, x.Errorf("Invalid query")
	}
	item := it.Item()
	if item.Typ != itemName {
		return nil, x.Errorf("Expected some name. Got: %v", item)
	}

	peekIt, err := it.Peek(1)
	if err != nil {
		return nil, x.Errorf("Invalid Query")
	}
	if peekIt[0].Typ == itemName && strings.ToLower(peekIt[0].Val) == "as" {
		gq.Var = item.Val
		it.Next() // Consume the "AS".
		it.Next()
		item = it.Item()
	}

	gq.Alias = item.Val
	if !it.Next() {
		return nil, x.Errorf("Invalid query")
	}
	item = it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected Left round brackets. Got: %v", item)
	}

	expectArg := true
	// Parse in KV fashion. Depending on the value of key, decide the path.
	for it.Next() {
		var key string
		// Get key.
		item := it.Item()
		if item.Typ == itemName {
			if !expectArg {
				return nil, x.Errorf("Expecting a comma. Got: %v", item)
			}
			key = item.Val
			expectArg = false
		} else if item.Typ == itemRightRound {
			break
		} else if item.Typ == itemComma {
			if expectArg {
				return nil, x.Errorf("Expected Argument but got comma.")
			}
			expectArg = true
			continue
		} else {
			return nil, x.Errorf("Expecting argument name. Got: %v", item)
		}

		if !it.Next() {
			return nil, x.Errorf("Invalid query")
		}
		item = it.Item()
		if item.Typ != itemColon {
			return nil, x.Errorf("Expecting a colon. Got: %v", item)
		}

		if key == "id" {
			if !it.Next() {
				return nil, x.Errorf("Invalid query")
			}
			item = it.Item()
			if item.Val == "var" {
				// Any number of variables allowed here.
				_, err := parseVarList(it, gq)
				if err != nil {
					return nil, err
				}
				continue
			}
			// Check and parse if its a list.
			err := parseID(gq, item.Val)
			if err != nil {
				return nil, err
			}
		} else if key == "func" {
			// Store the generator function.
			gen, err := parseFunction(it)
			if err != nil {
				return gq, err
			}
			gq.Func = gen
		} else {
			var val string
			if !it.Next() {
				return nil, x.Errorf("Invalid query")
			}
			item := it.Item()
			if item.Val == "var" {
				count, err := parseVarList(it, gq)
				if err != nil {
					return nil, err
				}
				if count != 1 {
					return nil, x.Errorf("Expected only one variable but got: %d", count)
				}
			} else {
				val = item.Val
			}
			if val == "" {
				val = gq.NeedsVar[len(gq.NeedsVar)-1]
			}
			gq.Args[key] = val
		}
	}

	return gq, nil
}

// godeep constructs the subgraph from the lexed items and a GraphQuery node.
func godeep(it *lex.ItemIterator, gq *GraphQuery) error {
	if gq == nil {
		return x.Errorf("Bad nesting of predicates or functions")
	}
	var isCount uint16
	varName := ""
	curp := gq // Used to track current node, for nesting.
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return x.Errorf(item.Val)
		case lex.ItemEOF:
			return nil
		case itemRightCurl:
			return nil
		case itemThreeDots:
			it.Next()
			item = it.Item()
			if item.Typ == itemName {
				// item.Val is expected to start with "..." and to have len >3.
				gq.Children = append(gq.Children, &GraphQuery{fragment: item.Val})
				// Unlike itemName, there is no nesting, so do not change "curp".
			}
		case itemName:
			// Handle count.
			peekIt, err := it.Peek(1)
			if err != nil {
				return x.Errorf("Invalid query")
			}

			if peekIt[0].Typ == itemName && strings.ToLower(peekIt[0].Val) == "as" {
				varName = item.Val
				it.Next() // "As" was checked before.
				continue
			}

			val := strings.ToLower(item.Val)
			if isAggregator(val) || val == "checkpwd" {
				item.Val = val
				child := &GraphQuery{
					Args: make(map[string]string),
					Var:  varName,
				}
				varName = ""
				it.Prev()
				if child.Func, err = parseFunction(it); err != nil {
					return err
				}
				if item.Val == "checkpwd" {
					child.Func.Args = append(child.Func.Args, child.Func.Attr)
				}
				child.Attr = child.Func.Attr
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			} else if isValVarFunc(val) {
				item.Val = val
				if varName == "" {
					return x.Errorf("Function %v should be used with a variable", val)
				}
				child := &GraphQuery{
					Attr:       item.Val,
					Args:       make(map[string]string),
					Var:        varName,
					IsInternal: true,
				}
				varName = ""
				count, err := parseVarList(it, child)
				if err != nil {
					return err
				}
				if count == 0 {
					return x.Errorf("Should have atleast one variable inside %v", val)
				}
				gq.Children = append(gq.Children, child)
				curp = nil
				continue

			} else if val == "count" {
				if isCount != 0 {
					return x.Errorf("Invalid mention of function count")
				}
				isCount = 1
				it.Next()
				item = it.Item()
				if item.Typ != itemLeftRound {
					return x.Errorf("Invalid mention of count.")
				}
				continue
			} else if val == "var" {
				if varName != "" {
					return x.Errorf("Cannot assign a variable to var()")
				}
				child := &GraphQuery{
					Attr:       val,
					Args:       make(map[string]string),
					IsInternal: true,
				}
				count, err := parseVarList(it, child)
				if err != nil {
					return err
				}
				if count != 1 {
					return x.Errorf("Invalid use of var(). Exactly one variable expected.")
				}
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			}
			if isCount == 2 {
				return x.Errorf("Multiple predicates not allowed in single count.")
			}
			child := &GraphQuery{
				Args:    make(map[string]string),
				Attr:    item.Val,
				IsCount: isCount == 1,
				Var:     varName,
			}
			varName = ""
			gq.Children = append(gq.Children, child)
			curp = child
			if isCount == 1 {
				isCount = 2
			}
		case itemColon:
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return x.Errorf("Predicate Expected but got: %s", item.Val)
			}
			curp.Alias = curp.Attr
			curp.Attr = item.Val
		case itemLeftCurl:
			if err := godeep(it, curp); err != nil {
				return err
			}
		case itemLeftRound:
			if curp.Attr == "" {
				return x.Errorf("Predicate name cannot be empty.")
			}
			args, err := parseArguments(it, curp)
			if err != nil {
				return err
			}
			// Stores args in GraphQuery, will be used later while retrieving results.
			for _, p := range args {
				if p.Val == "" {
					return x.Errorf("Got empty argument")
				}
				curp.Args[p.Key] = p.Val
			}
		case itemAt:
			err := parseDirective(it, curp)
			if err != nil {
				return err
			}
		case itemRightRound:
			if isCount != 2 {
				return x.Errorf("Invalid mention of brackets")
			}
			isCount = 0
		}
	}
	return nil
}

func isAggregator(fname string) bool {
	return fname == "min" || fname == "max" || fname == "sum" || fname == "avg"
}

func isValVarFunc(name string) bool {
	return name == "sumvar"
}
