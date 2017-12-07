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
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

const (
	uid   = "uid"
	value = "val"
)

// GraphQuery stores the parsed Query in a tree format. This gets converted to
// intern.y used query.SubGraph before processing the query.
type GraphQuery struct {
	UID        []uint64
	Attr       string
	Langs      []string
	Alias      string
	IsCount    bool
	IsInternal bool
	IsGroupby  bool
	Var        string
	NeedsVar   []VarContext
	Func       *Function
	Expand     string // Which variable to expand with.

	Args map[string]string
	// Query can have multiple sort parameters.
	Order        []*intern.Order
	Children     []*GraphQuery
	Filter       *FilterTree
	MathExp      *MathTree
	Normalize    bool
	Recurse      bool
	RecurseArgs  RecurseArgs
	Cascade      bool
	IgnoreReflex bool
	Facets       *intern.FacetParams
	FacetsFilter *FilterTree
	GroupbyAttrs []GroupByAttr
	FacetVar     map[string]string
	FacetOrder   string
	FacetDesc    bool

	// Internal fields below.
	// If gq.fragment is nonempty, then it is a fragment reference / spread.
	fragment string

	// Indicates whether count of uids is requested as a child node. If there
	// is an alias, then UidCountAlias will be set (otherwise it will be the
	// empty string).
	UidCount      bool
	UidCountAlias string

	// True for blocks that don't have a starting function and hence no starting nodes. They are
	// used to aggregate and get variables defined in another block.
	IsEmpty bool
}

type RecurseArgs struct {
	Depth     uint64
	AllowLoop bool
}

type GroupByAttr struct {
	Attr  string
	Alias string
	Langs []string
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

const (
	ANY_VAR   = 0
	UID_VAR   = 1
	VALUE_VAR = 2
	LIST_VAR  = 3
)

type VarContext struct {
	Name string
	Typ  int //  1 for UID vars, 2 for value vars
}

// varInfo holds information on GQL variables.
type varInfo struct {
	Value string
	Type  string
}

// varMap is a map with key as GQL variable name.
type varMap map[string]varInfo

// FilterTree is the result of parsing the filter directive.
// Either you can have `Op and Children` on non-leaf nodes
// Or Func at leaf nodes.
type FilterTree struct {
	Op    string
	Child []*FilterTree
	Func  *Function
}

type Arg struct {
	Value        string
	IsValueVar   bool // If argument is val(a)
	IsGraphQLVar bool
}

// Function holds the information about gql functions.
type Function struct {
	Attr       string
	Lang       string // language of the attribute value
	Name       string // Specifies the name of the function.
	Args       []Arg  // Contains the arguments of the function.
	UID        []uint64
	NeedsVar   []VarContext // If the function requires some variable
	IsCount    bool         // gt(count(friends),0)
	IsValueVar bool         // eq(val(s), 5)
}

// filterOpPrecedence is a map from filterOp (a string) to its precedence.
var filterOpPrecedence map[string]int
var mathOpPrecedence map[string]int

func init() {
	filterOpPrecedence = map[string]int{
		"not": 3,
		"and": 2,
		"or":  1,
	}
	mathOpPrecedence = map[string]int{
		"u-":      500,
		"floor":   105,
		"ceil":    104,
		"since":   103,
		"exp":     100,
		"ln":      99,
		"sqrt":    98,
		"cond":    90,
		"pow":     89,
		"logbase": 88,
		"max":     85,
		"min":     84,

		"/": 50,
		"*": 49,
		"%": 48,
		"-": 47,
		"+": 46,

		"<":  10,
		">":  9,
		"<=": 8,
		">=": 7,
		"==": 6,
		"!=": 5,
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

func convertToVarMap(variables map[string]string) (vm varMap) {
	vm = make(map[string]varInfo)
	for k, v := range variables {
		vm[k] = varInfo{
			Value: v,
		}
	}
	return vm
}

type Request struct {
	Str       string
	Variables map[string]string
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

func substituteVar(f string, res *string, vmap varMap) error {
	if len(f) > 0 && f[0] == '$' {
		va, ok := vmap[f]
		if !ok || va.Type == "" {
			return x.Errorf("Variable not defined %v", f)
		}
		*res = va.Value
	}
	return nil
}

func substituteVariables(gq *GraphQuery, vmap varMap) error {
	for k, v := range gq.Args {
		// v won't be empty as its handled in parseGqlVariables.
		val := gq.Args[k]
		if err := substituteVar(v, &val, vmap); err != nil {
			return err
		}
		gq.Args[k] = val
	}

	idVal, ok := gq.Args["id"]
	if ok && len(gq.UID) == 0 {
		if idVal == "" {
			return x.Errorf("Id can't be empty")
		}
		parseID(gq, idVal)
		// Deleting it here because we don't need to fill it in query.go.
		delete(gq.Args, "id")
	}

	if gq.Func != nil {
		if err := substituteVar(gq.Func.Attr, &gq.Func.Attr, vmap); err != nil {
			return err
		}

		for idx, v := range gq.Func.Args {
			if !v.IsGraphQLVar {
				continue
			}
			if err := substituteVar(v.Value, &gq.Func.Args[idx].Value, vmap); err != nil {
				return err
			}
		}
	}

	for _, child := range gq.Children {
		if err := substituteVariables(child, vmap); err != nil {
			return err
		}
	}
	if gq.Filter != nil {
		if err := substituteVariablesFilter(gq.Filter, vmap); err != nil {
			return err
		}
	}
	return nil
}

func substituteVariablesFilter(f *FilterTree, vmap varMap) error {
	if f.Func != nil {
		if err := substituteVar(f.Func.Attr, &f.Func.Attr, vmap); err != nil {
			return err
		}

		for idx, v := range f.Func.Args {
			if err := substituteVar(v.Value, &f.Func.Args[idx].Value, vmap); err != nil {
				return err
			}
		}
	}

	for _, fChild := range f.Child {
		if err := substituteVariablesFilter(fChild, vmap); err != nil {
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
	Schema    *intern.SchemaRequest
}

// Parse initializes and runs the lexer. It also constructs the GraphQuery subgraph
// from the lexed items.
func Parse(r Request) (res Result, rerr error) {
	query := r.Str
	vmap := convertToVarMap(r.Variables)

	lexer := lex.Lexer{Input: query}
	lexer.Run(lexTopLevel)

	var qu *GraphQuery
	it := lexer.NewIterator()
	fmap := make(fragmentMap)
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return res, x.Errorf(item.Val)

		case itemOpType:
			if item.Val == "mutation" {
				return res, x.Errorf("Mutation block no longer allowed.")
			}
			if item.Val == "schema" {
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

		allVars := res.QueryVars
		if err := checkDependency(allVars); err != nil {
			return res, err
		}
	}

	return res, nil
}

func flatten(vl []*Vars) (needs []string, defines []string) {
	needs, defines = make([]string, 0, 10), make([]string, 0, 10)
	for _, it := range vl {
		needs = append(needs, it.Needs...)
		defines = append(defines, it.Defines...)
	}
	return
}

func checkDependency(vl []*Vars) error {
	needs, defines := flatten(vl)

	needs = x.RemoveDuplicates(needs)
	lenBefore := len(defines)
	defines = x.RemoveDuplicates(defines)

	if len(defines) != lenBefore {
		return x.Errorf("Some variables are declared multiple times.")
	}

	if len(defines) > len(needs) {
		return x.Errorf("Some variables are defined but not used\nDefined:%v\nUsed:%v\n",
			defines, needs)
	}

	if len(defines) < len(needs) {
		return x.Errorf("Some variables are used but not defined\nDefined:%v\nUsed:%v\n",
			defines, needs)
	}

	for i := 0; i < len(defines); i++ {
		if defines[i] != needs[i] {
			return x.Errorf("Variables are not used properly. \nDefined:%v\nUsed:%v\n",
				defines, needs)
		}
	}
	return nil
}

func (qu *GraphQuery) collectVars(v *Vars) {
	if qu.Var != "" {
		v.Defines = append(v.Defines, qu.Var)
	}
	if qu.FacetVar != nil {
		for _, va := range qu.FacetVar {
			v.Defines = append(v.Defines, va)
		}
	}
	for _, va := range qu.NeedsVar {
		v.Needs = append(v.Needs, va.Name)
	}

	for _, ch := range qu.Children {
		ch.collectVars(v)
	}
	if qu.Filter != nil {
		qu.Filter.collectVars(v)
	}
	if qu.MathExp != nil {
		qu.MathExp.collectVars(v)
	}
}

func (f *MathTree) collectVars(v *Vars) {
	if f == nil {
		return
	}
	if f.Var != "" {
		v.Needs = append(v.Needs, f.Var)
		return
	}
	for _, fch := range f.Child {
		fch.collectVars(v)
	}
}

func (f *FilterTree) collectVars(v *Vars) {
	if f.Func != nil {
		for _, va := range f.Func.NeedsVar {
			v.Needs = append(v.Needs, va.Name)
		}
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
func getVariablesAndQuery(it *lex.ItemIterator, vmap varMap) (gq *GraphQuery, rerr error) {
	var name string
L2:
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return nil, x.Errorf(item.Val)
		case itemName:
			if name != "" {
				return nil, x.Errorf("Multiple word query name not allowed.")
			}
			name = item.Val
		case itemLeftRound:
			if name == "" {
				return nil, x.Errorf("Variables can be defined only in named queries.")
			}

			if rerr = parseGqlVariables(it, vmap); rerr != nil {
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

func parseRecurseArgs(it *lex.ItemIterator, gq *GraphQuery) error {
	if ok := trySkipItemTyp(it, itemLeftRound); !ok {
		// We don't have a (, we can return.
		return nil
	}

	var key, val string
	var item lex.Item
	var ok bool
	for it.Next() {
		item = it.Item()
		if item.Typ != itemName {
			return fmt.Errorf("Expected key inside @recurse().")
		}
		key = strings.ToLower(item.Val)

		if ok := trySkipItemTyp(it, itemColon); !ok {
			return fmt.Errorf("Expected colon(:) after %s")
		}

		if item, ok = tryParseItemType(it, itemName); !ok {
			return fmt.Errorf("Expected value inside @recurse() for key: %s.", key)
		}
		val = item.Val

		switch key {
		case "depth":
			depth, err := strconv.ParseUint(val, 0, 64)
			if err != nil {
				return err
			}
			gq.RecurseArgs.Depth = depth
		case "loop":
			allowLoop, err := strconv.ParseBool(val)
			if err != nil {
				return err
			}
			gq.RecurseArgs.AllowLoop = allowLoop
		default:
			return fmt.Errorf("Unexpected key: [%s] inside @recurse block", key)
		}

		if _, ok := tryParseItemType(it, itemRightRound); ok {
			return nil
		}

		if _, ok := tryParseItemType(it, itemComma); !ok {
			return fmt.Errorf("Expected comma after value: %s inside recurse block.", val)
		}
	}
	return nil
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
			switch strings.ToLower(item.Val) {
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
			case "cascade":
				gq.Cascade = true
			case "groupby":
				gq.IsGroupby = true
				parseGroupby(it, gq)
			case "ignorereflex":
				gq.IgnoreReflex = true
			case "recurse":
				gq.Recurse = true
				if err := parseRecurseArgs(it, gq); err != nil {
					return nil, err
				}
			default:
				return nil, x.Errorf("Unknown directive [%s]", item.Val)
			}
			goto L
		}
	} else if item.Typ == itemRightCurl {
		// Do nothing.
	} else if item.Typ == itemName {
		it.Prev()
		return gq, nil
	} else {
		return nil, x.Errorf("Malformed Query. Missing {. Got %v", item.Val)
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
			val := collectName(it, item.Val)
			items = append(items, val)
		case itemComma:
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return items, x.Errorf("Invalid scheam block")
			}
			val := collectName(it, item.Val)
			items = append(items, val)
		default:
			return items, x.Errorf("Invalid schema block")
		}
	}
	return items, x.Errorf("Invalid schema block")
}

// parses till rightround is found
func parseSchemaPredicates(it *lex.ItemIterator, s *intern.SchemaRequest) error {
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
func parseSchemaFields(it *lex.ItemIterator, s *intern.SchemaRequest) error {
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

func getSchema(it *lex.ItemIterator) (*intern.SchemaRequest, error) {
	var s intern.SchemaRequest
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

// parseGqlVariables parses the the graphQL variable declaration.
func parseGqlVariables(it *lex.ItemIterator, vmap varMap) error {
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
			return x.Errorf("Expecting a colon. Got: %v", item)
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
		it.Next()
		item = it.Item()
		if item.Typ == itemMathOp && item.Val == "!" {
			varType += item.Val
			it.Next()
			item = it.Item()
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
				uq, err := unquoteIfQuoted(it.Val)
				if err != nil {
					return err
				}
				vmap[varName] = varInfo{
					Value: uq,
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

// unquoteIfQuoted checks if str is quoted (starts and ends with quotes). If
// so, it tries to unquote str possibly returning an error. Otherwise, the
// original value is returned.
func unquoteIfQuoted(str string) (string, error) {
	if len(str) < 2 || str[0] != quote || str[len(str)-1] != quote {
		return str, nil
	}
	uq, err := strconv.Unquote(str)
	return uq, x.Wrapf(err, "could not unquote %q:", str)
}

// parseArguments parses the arguments part of the GraphQL query root.
func parseArguments(it *lex.ItemIterator, gq *GraphQuery) (result []pair, rerr error) {
	expectArg := true
	orderCount := 0
	for it.Next() {
		var p pair
		// Get key.
		item := it.Item()
		if item.Typ == itemName {
			if !expectArg {
				return result, x.Errorf("Expecting a comma. But got: %v", item.Val)
			}
			p.Key = collectName(it, item.Val)
			if isSortkey(p.Key) {
				orderCount++
			}
			expectArg = false
		} else if item.Typ == itemRightRound {
			if expectArg {
				return result, x.Errorf("Expected argument but got ')'.")
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
		if item.Val == value {
			count, err := parseVarList(it, gq)
			if err != nil {
				return result, err
			}
			if count != 1 {
				return result, x.Errorf("Only one variable expected. Got %d", count)
			}
			gq.NeedsVar[len(gq.NeedsVar)-1].Typ = VALUE_VAR
			p.Val = gq.NeedsVar[len(gq.NeedsVar)-1].Name
			result = append(result, p)
			if isSortkey(p.Key) && orderCount > 1 {
				return result, x.Errorf("Multiple sorting only allowed by predicates. Got: %+v", p.Val)
			}
			continue
		}

		if item.Typ == itemDollar {
			val = "$"
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return result, x.Errorf("Expecting argument value. Got: %v", item)
			}
		} else if item.Typ == itemMathOp {
			if item.Val != "+" && item.Val != "-" {
				return result, x.Errorf("Only Plus and minus are allowed unary ops. Got: %v", item.Val)
			}
			val = item.Val
			it.Next()
			item = it.Item()
		} else if item.Typ != itemName {
			return result, x.Errorf("Expecting argument value. Got: %v", item)
		}

		p.Val = collectName(it, val+item.Val)

		// Get language list, if present
		items, err := it.Peek(1)
		if err == nil && items[0].Typ == itemAt {
			it.Next() // consume '@'
			it.Next() // move forward
			langs, err := parseLanguageList(it)
			if err != nil {
				return nil, err
			}
			p.Val = p.Val + "@" + strings.Join(langs, ":")
		}

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
			if t.Func.IsCount {
				buf.WriteString("count(")
			}
			buf.WriteString(t.Func.Attr)
			if t.Func.IsCount {
				buf.WriteRune(')')
			}
			if len(t.Func.Lang) > 0 {
				buf.WriteRune('@')
				buf.WriteString(t.Func.Lang)
			}

			for _, arg := range t.Func.Args {
				if arg.IsValueVar {
					buf.WriteString(" val(")
				} else {
					buf.WriteString(" \"")
				}
				buf.WriteString(arg.Value)
				if arg.IsValueVar {
					buf.WriteRune(')')
				} else {
					buf.WriteRune('"')
				}
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

func (s *filterTreeStack) popAssert() *FilterTree {
	x.AssertTrue(!s.empty())
	last := s.a[len(s.a)-1]
	s.a = s.a[:len(s.a)-1]
	return last
}

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
		topVal1 := valueStack.popAssert()
		topVal2 := valueStack.popAssert()
		topOp.Child = []*FilterTree{topVal2, topVal1}
	}
	// Push the new value (tree) into the valueStack.
	valueStack.push(topOp)
	return nil
}

func parseGeoArgs(it *lex.ItemIterator, g *Function) error {
	buf := new(bytes.Buffer)
	buf.WriteString("[")
	depth := 1
	for {
		if valid := it.Next(); !valid {
			return x.Errorf("Got EOF while parsing Geo tokens")
		}
		item := it.Item()
		switch item.Typ {
		case itemLeftSquare:
			buf.WriteString(item.Val)
			depth++
		case itemRightSquare:
			buf.WriteString(item.Val)
			depth--
		case itemMathOp, itemComma, itemName:
			// Writing tokens to buffer.
			buf.WriteString(item.Val)
		default:
			return x.Errorf("Found invalid item: %s while parsing geo arguments.",
				item.Val)
		}

		if depth > 4 || depth < 0 {
			return x.Errorf("Invalid bracket sequence")
		} else if depth == 0 {
			break
		}
	}
	// Lets append the concatenated Geo token to Args.
	// TODO - See if we can directly encode to Geo format.
	g.Args = append(g.Args, Arg{Value: buf.String()})
	items, err := it.Peek(1)
	if err != nil {
		return x.Errorf("Unexpected EOF while parsing args")
	}
	item := items[0]
	if item.Typ != itemRightRound && item.Typ != itemComma {
		return x.Errorf("Expected right round or comma. Got: %+v",
			items[0])
	}
	return nil
}

func validFuncName(name string) bool {
	if isGeoFunc(name) || isInequalityFn(name) {
		return true
	}

	switch name {
	case "regexp", "anyofterms", "allofterms", "alloftext", "anyoftext",
		"has", "uid", "uid_in", "anyof", "allof":
		return true
	}
	return false
}

func parseFunction(it *lex.ItemIterator, gq *GraphQuery) (*Function, error) {
	var function *Function
	var expectArg, seenFuncArg, expectLang, isDollar bool
L:
	for it.Next() {
		item := it.Item()
		if item.Typ != itemName {
			return nil, x.Errorf("Expected a function but got %q", item.Val)

		}

		val := collectName(it, item.Val)
		function = &Function{
			Name: strings.ToLower(val),
		}
		if _, ok := tryParseItemType(it, itemLeftRound); !ok {
			return nil, x.Errorf("Expected ( after func name [%s]", function.Name)
		}

		attrItemsAgo := -1
		expectArg = true
		for it.Next() {
			itemInFunc := it.Item()
			if attrItemsAgo >= 0 {
				attrItemsAgo++
			}
			var val string
			if itemInFunc.Typ == itemRightRound {
				break L
			} else if itemInFunc.Typ == itemComma {
				if expectArg {
					return nil, x.Errorf("Invalid use of comma.")
				}
				if isDollar {
					return nil, x.Errorf("Invalid use of comma after dollar.")
				}
				expectArg = true
				continue
			} else if itemInFunc.Typ == itemLeftRound {
				// Function inside a function.
				if seenFuncArg {
					return nil, x.Errorf("Multiple functions as arguments not allowed")
				}
				it.Prev()
				it.Prev()
				nestedFunc, err := parseFunction(it, gq)
				if err != nil {
					return nil, err
				}
				seenFuncArg = true
				if nestedFunc.Name == value {
					if len(nestedFunc.NeedsVar) > 1 {
						return nil, x.Errorf("Multiple variables not allowed in a function")
					}
					// Variable is used in place of attribute, eq(val(a), 5)
					if len(function.Attr) == 0 {
						function.Attr = nestedFunc.NeedsVar[0].Name
						function.IsValueVar = true
					} else {
						// eq(name, val(a))
						function.Args = append(function.Args, Arg{Value: nestedFunc.NeedsVar[0].Name, IsValueVar: true})
					}
					function.NeedsVar = append(function.NeedsVar, nestedFunc.NeedsVar...)
					function.NeedsVar[0].Typ = VALUE_VAR
				} else {
					if nestedFunc.Name != "count" {
						return nil,
							x.Errorf("Only val/count allowed as function within another. Got: %s", nestedFunc.Name)
					}
					function.Attr = nestedFunc.Attr
					function.IsCount = true
				}
				expectArg = false
				continue
			} else if itemInFunc.Typ == itemAt {
				if attrItemsAgo != 1 {
					return nil, x.Errorf("Invalid usage of '@' in function " +
						"argument, must only appear immediately after attr.")
				}
				expectLang = true
				continue
			} else if itemInFunc.Typ == itemMathOp {
				val = itemInFunc.Val
				it.Next()
				itemInFunc = it.Item()
			} else if itemInFunc.Typ == itemDollar {
				if isDollar {
					return nil, x.Errorf("Invalid use of $ in func args")
				}
				isDollar = true
				continue
			} else if itemInFunc.Typ == itemRegex {
				end := strings.LastIndex(itemInFunc.Val, "/")
				x.AssertTrue(end >= 0)
				expr := strings.Replace(itemInFunc.Val[1:end], "\\/", "/", -1)
				flags := ""
				if end+1 < len(itemInFunc.Val) {
					flags = itemInFunc.Val[end+1:]
				}

				function.Args = append(function.Args, Arg{Value: expr}, Arg{Value: flags})
				expectArg = false
				continue
				// Lets reassemble the geo tokens.
			} else if itemInFunc.Typ == itemLeftSquare {
				isGeo := isGeoFunc(function.Name)
				if !isGeo && !isInequalityFn(function.Name) {
					return nil, x.Errorf("Unexpected character [ while parsing request.")
				}

				if isGeo {
					if err := parseGeoArgs(it, function); err != nil {
						return nil, err
					}
					expectArg = false
					continue
				}

				if valid := it.Next(); !valid {
					return nil,
						x.Errorf("Unexpected EOF while parsing args")
				}
				itemInFunc = it.Item()
			} else if itemInFunc.Typ == itemRightSquare {
				if _, err := it.Peek(1); err != nil {
					return nil,
						x.Errorf("Unexpected EOF while parsing args")
				}
				expectArg = false
				continue
			} else if itemInFunc.Typ != itemName {
				return nil, x.Errorf("Expected arg after func [%s], but got item %v",
					function.Name, itemInFunc)
			}

			item, ok := it.PeekOne()
			// Part of function continue
			if ok && item.Typ == itemLeftRound {
				continue
			}

			if !expectArg && !expectLang {
				return nil, x.Errorf("Expected comma or language but got: %s", itemInFunc.Val)
			}

			// TODO - Move this to a function.
			v := strings.Trim(itemInFunc.Val, " \t")
			var err error
			v, err = unquoteIfQuoted(v)
			if err != nil {
				return nil, err
			}
			val += v
			if val == "" {
				return nil, x.Errorf("Empty argument received")
			}
			if val == "uid" {
				return nil, x.Errorf("Argument cannot be %q", val)
			}

			if isDollar {
				val = "$" + val
				isDollar = false
				if function.Name == uid && gq != nil {
					if len(gq.Args["id"]) > 0 {
						return nil,
							x.Errorf("Only one GraphQL variable allowed inside uid function.")
					}
					gq.Args["id"] = val
				} else {
					function.Args = append(function.Args, Arg{Value: val, IsGraphQLVar: true})
				}
				expectArg = false
				continue
			}

			// Unlike other functions, uid function has no attribute, everything is args.
			if len(function.Attr) == 0 && function.Name != "uid" {
				if strings.ContainsRune(itemInFunc.Val, '"') {
					return nil, x.Errorf("Attribute in function must not be quoted with \": %s",
						itemInFunc.Val)
				}
				function.Attr = val
				attrItemsAgo = 0
			} else if expectLang {
				function.Lang = val
				expectLang = false
			} else if function.Name != uid {
				// For UID function. we set g.UID
				function.Args = append(function.Args, Arg{Value: val})
			}

			if function.Name == "var" {
				return nil, x.Errorf("Unexpected var(). Maybe you want to try using uid()")
			}

			expectArg = false
			if function.Name == value {
				// E.g. @filter(gt(val(a), 10))
				function.NeedsVar = append(function.NeedsVar, VarContext{
					Name: val,
					Typ:  VALUE_VAR,
				})
			} else if function.Name == uid {
				// uid function could take variables as well as actual uids.
				// If we can parse the value that means its an uid otherwise a variable.
				uid, err := strconv.ParseUint(val, 0, 64)
				if err == nil {
					// It could be uid function at root.
					if gq != nil {
						gq.UID = append(gq.UID, uid)
						// Or uid function in filter.
					} else {
						function.UID = append(function.UID, uid)
					}
					continue
				}
				// E.g. @filter(uid(a, b, c))
				function.NeedsVar = append(function.NeedsVar, VarContext{
					Name: val,
					Typ:  UID_VAR,
				})
			}
		}
	}

	if function.Name != uid && len(function.Attr) == 0 {
		return nil, x.Errorf("Got empty attr for function: [%s]", function.Name)
	}

	return function, nil
}

type facetRes struct {
	f          *intern.FacetParams
	ft         *FilterTree
	vmap       map[string]string
	facetOrder string
	orderdesc  bool
}

func parseFacets(it *lex.ItemIterator) (res facetRes, err error) {
	res, ok, err := tryParseFacetList(it)
	if err != nil || ok {
		return res, err
	}

	filterTree, err := parseFilter(it)
	res.ft = filterTree
	return res, err
}

type facetItem struct {
	name      string
	alias     string
	varName   string
	ordered   bool
	orderdesc bool
}

// If err != nil, an error happened, abandon parsing.  If err == nil && parseOk == false, the
// attempt to parse failed, no data was consumed, and there might be a valid alternate parse with a
// different function.
func tryParseFacetItem(it *lex.ItemIterator) (res facetItem, parseOk bool, err error) {
	// We parse this:
	// [{orderdesc|orderasc|alias}:] [varname as] name

	savePos := it.Save()
	defer func() {
		if err == nil && !parseOk {
			it.Restore(savePos)
		}
	}()

	item, ok := tryParseItemType(it, itemName)
	if !ok {
		return res, false, nil
	}

	isOrderasc := item.Val == "orderasc"
	if _, ok := tryParseItemType(it, itemColon); ok {
		if isOrderasc || item.Val == "orderdesc" {
			res.ordered = true
			res.orderdesc = !isOrderasc
		} else {
			res.alias = item.Val
		}

		// Step past colon.
		item, ok = tryParseItemType(it, itemName)
		if !ok {
			return res, false, x.Errorf("Expected name after colon")
		}
	}

	// We've possibly set ordered, orderdesc, alias and now we have consumed another item,
	// which is a name.
	name1 := item.Val

	// Now try to consume "as".
	if !trySkipItemVal(it, "as") {
		name1 = collectName(it, name1)
		res.name = name1
		return res, true, nil
	}
	item, ok = tryParseItemType(it, itemName)
	if !ok {
		return res, false, x.Errorf("Expected name in facet list")
	}

	res.name = collectName(it, item.Val)
	res.varName = name1
	return res, true, nil
}

// If err != nil, an error happened, abandon parsing.  If err == nil && parseOk == false, the
// attempt to parse failed, but there might be a valid alternate parse with a different function,
// such as parseFilter.
func tryParseFacetList(it *lex.ItemIterator) (res facetRes, parseOk bool, err error) {
	savePos := it.Save()
	defer func() {
		if err == nil && !parseOk {
			it.Restore(savePos)
		}
	}()

	// Skip past '('
	if _, ok := tryParseItemType(it, itemLeftRound); !ok {
		it.Restore(savePos)
		var facets intern.FacetParams
		facets.AllKeys = true
		res.f = &facets
		res.vmap = make(map[string]string)
		return res, true, nil
	}

	facetVar := make(map[string]string)
	var facets intern.FacetParams
	var orderdesc bool
	var orderkey string

	if _, ok := tryParseItemType(it, itemRightRound); ok {
		// @facets() just parses to an empty set of facets.
		res.f, res.vmap, res.facetOrder, res.orderdesc = &facets, facetVar, orderkey, orderdesc
		return res, true, nil
	}

	for {
		// We've just consumed a leftRound or a comma.

		// Parse a facet item.
		facetItem, ok, err := tryParseFacetItem(it)
		if !ok || err != nil {
			return res, ok, err
		}

		// Combine the facetitem with our result.
		{
			if facetItem.varName != "" {
				if _, has := facetVar[facetItem.name]; has {
					return res, false, x.Errorf("Duplicate variable mappings for facet %v",
						facetItem.name)
				}
				facetVar[facetItem.name] = facetItem.varName
			}
			facets.Param = append(facets.Param, &intern.FacetParam{
				Key:   facetItem.name,
				Alias: facetItem.alias,
			})
			if facetItem.ordered {
				if orderkey != "" {
					return res, false, x.Errorf("Invalid use of orderasc/orderdesc in facets")
				}
				orderdesc = facetItem.orderdesc
				orderkey = facetItem.name
			}
		}

		// Now what?  Either close-paren or a comma.
		if _, ok := tryParseItemType(it, itemRightRound); ok {
			sort.Slice(facets.Param, func(i, j int) bool {
				return facets.Param[i].Key < facets.Param[j].Key
			})
			// deduplicate facets
			out := facets.Param[:0]
			flen := len(facets.Param)
			for i := 1; i < flen; i++ {
				if facets.Param[i-1].Key == facets.Param[i].Key {
					continue
				}
				out = append(out, facets.Param[i-1])
			}
			out = append(out, facets.Param[flen-1])
			facets.Param = out
			res.f, res.vmap, res.facetOrder, res.orderdesc = &facets, facetVar, orderkey, orderdesc
			return res, true, nil
		}
		if item, ok := tryParseItemType(it, itemComma); !ok {
			if len(facets.Param) < 2 {
				// We have only consumed ``'@facets' '(' <facetItem>`, which means parseFilter might
				// succeed. Return no-parse, no-error.
				return res, false, nil
			}
			// We've consumed `'@facets' '(' <facetItem> ',' <facetItem>`, so this is definitely
			// not a filter.  Return an error.
			return res, false, x.Errorf(
				"Expected ',' or ')' in facet list", item.Val)
		}
	}
}

// parseGroupby parses the groupby directive.
func parseGroupby(it *lex.ItemIterator, gq *GraphQuery) error {
	count := 0
	expectArg := true
	it.Next()
	item := it.Item()
	alias := ""
	if item.Typ != itemLeftRound {
		return x.Errorf("Expected a left round after groupby")
	}
	for it.Next() {
		item := it.Item()
		if item.Typ == itemRightRound {
			break
		}
		if item.Typ == itemComma {
			if expectArg {
				return x.Errorf("Expected a predicate but got comma")
			}
			expectArg = true
		} else if item.Typ == itemName {
			if !expectArg {
				return x.Errorf("Expected a comma or right round but got: %v", item.Val)
			}

			val := collectName(it, item.Val)
			peekIt, err := it.Peek(1)
			if err != nil {
				return err
			}
			if peekIt[0].Typ == itemColon {
				if alias != "" {
					return x.Errorf("Expected predicate after %s:", alias)
				}
				alias = val
				it.Next() // Consume the itemColon
				continue
			}

			var langs []string
			items, err := it.Peek(1)
			if err == nil && items[0].Typ == itemAt {
				it.Next() // consume '@'
				it.Next() // move forward
				langs, err = parseLanguageList(it)
				if err != nil {
					return err
				}
			}
			attrLang := GroupByAttr{
				Attr:  val,
				Alias: alias,
				Langs: langs,
			}
			alias = ""
			gq.GroupbyAttrs = append(gq.GroupbyAttrs, attrLang)
			count++
			expectArg = false
		}
	}
	if expectArg {
		return x.Errorf("Unnecessary comma in groupby()")
	}
	if count == 0 {
		return x.Errorf("Expected atleast one attribute in groupby")
	}
	return nil
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
			f, err := parseFunction(it, nil)
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
	if !opStack.empty() {
		return nil, x.Errorf("Unbalanced parentheses in @filter statement")
	}

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

// Parses ID list. Only used for GraphQL variables.
// TODO - Maybe get rid of this by lexing individual IDs.
func parseID(gq *GraphQuery, val string) error {
	val = x.WhiteSpace.Replace(val)
	if val[0] != '[' {
		uid, err := strconv.ParseUint(val, 0, 64)
		if err != nil {
			return err
		}
		gq.UID = append(gq.UID, uid)
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
			uid, err := strconv.ParseUint(buf.String(), 0, 64)
			if err != nil {
				return err
			}
			gq.UID = append(gq.UID, uid)
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
			gq.NeedsVar = append(gq.NeedsVar, VarContext{
				Name: item.Val,
				Typ:  UID_VAR,
			})
			expectArg = false
		}
	}
	if expectArg {
		return count, x.Errorf("Unnecessary comma in val()")
	}
	return count, nil
}

func parseDirective(it *lex.ItemIterator, curp *GraphQuery) error {
	valid := true
	it.Prev()
	item := it.Item()
	if item.Typ == itemLeftCurl {
		// Ideally we should check that curp was created at current depth.
		valid = false
	}
	it.Next()
	// No directive is allowed on intern.subgraph like expand all, value variables.
	if !valid || curp == nil || curp.IsInternal {
		return x.Errorf("Invalid use of directive.")
	}

	it.Next()
	item = it.Item()
	peek, err := it.Peek(1)
	if err != nil || item.Typ != itemName {
		return x.Errorf("Expected directive or language list")
	}

	if item.Val == "facets" { // because @facets can come w/t '()'
		res, err := parseFacets(it)
		if err != nil {
			return err
		}
		if res.f != nil {
			curp.FacetVar = res.vmap
			curp.FacetOrder = res.facetOrder
			curp.FacetDesc = res.orderdesc
			if curp.Facets != nil {
				return x.Errorf("Only one facets allowed")
			}
			curp.Facets = res.f
		} else if res.ft != nil {
			if curp.FacetsFilter != nil {
				return x.Errorf("Only one facets filter allowed")
			}
			if res.ft.hasVars() {
				return x.Errorf(
					"variables are not allowed in facets filter.")
			}
			curp.FacetsFilter = res.ft
		} else {
			return x.Errorf("Facets parsing failed.")
		}
	} else if peek[0].Typ == itemLeftRound {
		// this is directive
		switch item.Val {
		case "filter":
			if curp.Filter != nil {
				return x.Errorf("Use AND, OR and round brackets instead of multiple filter directives.")
			}
			filter, err := parseFilter(it)
			if err != nil {
				return err
			}
			curp.Filter = filter
		case "groupby":
			if curp.IsGroupby {
				return x.Errorf("Only one group by directive allowed.")
			}
			curp.IsGroupby = true
			parseGroupby(it, curp)
		default:
			return x.Errorf("Unknown directive [%s]", item.Val)
		}
	} else if len(curp.Attr) > 0 && len(curp.Langs) == 0 {
		// this is language list
		if curp.Langs, err = parseLanguageList(it); err != nil {
			return err
		}
		if len(curp.Langs) == 0 {
			return x.Errorf("Expected at least 1 language in list for %s", curp.Attr)
		}
	} else {
		return x.Errorf("Expected directive or language list, got @%s", item.Val)
	}
	return nil
}

func parseLanguageList(it *lex.ItemIterator) ([]string, error) {
	item := it.Item()
	var langs []string
	for ; item.Typ == itemName || item.Typ == itemPeriod; item = it.Item() {
		langs = append(langs, item.Val)
		it.Next()
		if it.Item().Typ == itemColon {
			it.Next()
		} else {
			break
		}
	}
	if it.Item().Typ == itemPeriod {
		peekIt, err := it.Peek(1)
		if err != nil {
			return nil, err
		}
		if peekIt[0].Typ == itemPeriod {
			return nil, x.Errorf("Expected only one dot(.) while parsing language list.")
		}
	}
	it.Prev()

	return langs, nil
}

func validKeyAtRoot(k string) bool {
	switch k {
	case "func", "orderasc", "orderdesc", "first", "offset", "after":
		return true
	case "from", "to", "numpaths":
		// Specific to shortest path
		return true
	case "depth":
		return true
	}
	return false
}

// Check for validity of key at non-root nodes.
func validKey(k string) bool {
	switch k {
	case "orderasc", "orderdesc", "first", "offset", "after":
		return true
	}
	return false
}

func attrAndLang(attrData string) (attr string, langs []string) {
	idx := strings.Index(attrData, "@")
	if idx < 0 {
		return attrData, langs
	}
	attr = attrData[:idx]
	langs = strings.Split(attrData[idx+1:], ":")
	return
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
	order := make(map[string]bool)
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
			if gq.Func == nil && len(gq.NeedsVar) == 0 && len(gq.Args) == 0 {
				// Used to do aggregation at root which would be fetched in another block.
				gq.IsEmpty = true
			}
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

		if !validKeyAtRoot(key) {
			return nil, x.Errorf("Got invalid keyword: %s at root", key)
		}

		if !it.Next() {
			return nil, x.Errorf("Invalid query")
		}
		item = it.Item()
		if item.Typ != itemColon {
			return nil, x.Errorf("Expecting a colon. Got: %v", item)
		}

		if key == "func" {
			// Store the generator function.
			if gq.Func != nil {
				return gq, x.Errorf("Only one function allowed at root")
			}
			gen, err := parseFunction(it, gq)
			if err != nil {
				return gq, err
			}
			if !validFuncName(gen.Name) {
				return nil, x.Errorf("Function name: %s is not valid.", gen.Name)
			}
			gq.Func = gen
			gq.NeedsVar = append(gq.NeedsVar, gen.NeedsVar...)
		} else {
			var val string
			if !it.Next() {
				return nil, x.Errorf("Invalid query")
			}
			item := it.Item()

			if item.Typ == itemMathOp {
				if item.Val != "+" && item.Val != "-" {
					return nil,
						x.Errorf("Only Plus and minus are allowed unary ops. Got: %v",
							item.Val)
				}
				val = item.Val
				it.Next()
				item = it.Item()
			}

			if val == "" && item.Val == value {
				count, err := parseVarList(it, gq)
				if err != nil {
					return nil, err
				}
				if count != 1 {
					return nil, x.Errorf("Expected only one variable but got: %d", count)
				}
				// Modify the NeedsVar context here.
				gq.NeedsVar[len(gq.NeedsVar)-1].Typ = VALUE_VAR
			} else {
				val = collectName(it, val+item.Val)
				// Get language list, if present
				items, err := it.Peek(1)
				if err == nil && items[0].Typ == itemLeftRound {
					if (key == "orderasc" || key == "orderdesc") && val != value {
						return nil, x.Errorf("Expected val(). Got %s() with order.", val)
					}
				}
				if err == nil && items[0].Typ == itemAt {
					it.Next() // consume '@'
					it.Next() // move forward
					langs, err := parseLanguageList(it)
					if err != nil {
						return nil, err
					}
					val = val + "@" + strings.Join(langs, ":")
				}

			}

			// TODO - Allow only order by one of variable/predicate for now.
			if val == "" {
				// Right now we only allow one sort by a variable and it has to be at the first
				// position.
				val = gq.NeedsVar[len(gq.NeedsVar)-1].Name
				if len(gq.Order) > 0 && isSortkey(key) {
					return nil, x.Errorf("Multiple sorting only allowed by predicates. Got: %+v", val)
				}
			}
			if isSortkey(key) {
				if order[val] {
					return nil, x.Errorf("Sorting by an attribute: [%s] can only be done once", val)
				}
				attr, langs := attrAndLang(val)
				gq.Order = append(gq.Order, &intern.Order{attr, key == "orderdesc", langs})
				order[val] = true
				continue
			}

			if _, ok := gq.Args[key]; ok {
				return gq, x.Errorf("Repeated key %q at root", key)
			}
			gq.Args[key] = val
		}
	}

	return gq, nil
}

func isSortkey(k string) bool {
	return k == "orderasc" || k == "orderdesc"
}

type Count int

const (
	notSeen      Count = iota // default value
	seen                      // when we see count keyword
	seenWithPred              // when we see a predicate within count.
)

func validateEmptyBlockItem(it *lex.ItemIterator, val string) error {
	savePos := it.Save()
	defer func() {
		it.Restore(savePos)
	}()

	fname := val
	// Could have alias so peek forward to get actual function name.
	skipped := trySkipItemTyp(it, itemColon)
	if skipped {
		item, ok := tryParseItemType(it, itemName)
		if !ok {
			return x.Errorf("Expected name. Got: %s", item.Val)
		}
		fname = item.Val
	}
	ok := trySkipItemTyp(it, itemLeftRound)
	if !ok || (!isMathBlock(fname) && !isAggregator(fname)) {
		return x.Errorf("Only aggregation/math functions allowed inside empty blocks."+
			" Got: %v", fname)
	}
	return nil
}

// godeep constructs the subgraph from the lexed items and a GraphQuery node.
func godeep(it *lex.ItemIterator, gq *GraphQuery) error {
	if gq == nil {
		return x.Errorf("Bad nesting of predicates or functions")
	}
	var count Count
	var alias, varName string
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
		case itemPeriod:
			// looking for ...
			dots := 1
			for i := 0; i < 2; i++ {
				if it.Next() && it.Item().Typ == itemPeriod {
					dots++
				}
			}
			if dots == 3 {
				it.Next()
				item = it.Item()
				if item.Typ == itemName {
					// item.Val is expected to start with "..." and to have len >3.
					gq.Children = append(gq.Children, &GraphQuery{fragment: item.Val})
					// Unlike itemName, there is no nesting, so do not change "curp".
				}
			} else {
				return x.Errorf("Expected 3 periods (\"...\"), got %d.", dots)
			}
		case itemName:
			peekIt, err := it.Peek(1)
			if err != nil {
				return x.Errorf("Invalid query")
			}
			if peekIt[0].Typ == itemName && strings.ToLower(peekIt[0].Val) == "as" {
				varName = item.Val
				it.Next() // "As" was checked before.
				continue
			}

			val := collectName(it, item.Val)
			valLower := strings.ToLower(val)

			peekIt, err = it.Peek(1)
			if err != nil {
				return err
			}
			if peekIt[0].Typ == itemColon {
				alias = val
				it.Next() // Consume the itemCollon
				continue
			}

			if gq.IsGroupby && (!isAggregator(val) && val != "count" && count != seen) {
				// Only aggregator or count allowed inside the groupby block.
				return x.Errorf("Only aggregator/count functions allowed inside @groupby. Got: %v", val)
			}

			if gq.IsEmpty {
				if err := validateEmptyBlockItem(it, valLower); err != nil {
					return err
				}
			}

			if valLower == "checkpwd" {
				child := &GraphQuery{
					Args:  make(map[string]string),
					Var:   varName,
					Alias: alias,
				}
				varName, alias = "", ""
				it.Prev()
				if child.Func, err = parseFunction(it, gq); err != nil {
					return err
				}
				child.Func.Args = append(child.Func.Args, Arg{Value: child.Func.Attr})
				child.Attr = child.Func.Attr
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			} else if isAggregator(valLower) {
				child := &GraphQuery{
					Attr:       value,
					Args:       make(map[string]string),
					Var:        varName,
					IsInternal: true,
					Alias:      alias,
				}
				varName, alias = "", ""
				it.Next()
				if it.Item().Typ != itemLeftRound {
					it.Prev()
					goto Fall
				}
				it.Next()
				if gq.IsGroupby {
					item = it.Item()
					attr := collectName(it, item.Val)
					// Get language list, if present
					items, err := it.Peek(1)
					if err == nil && items[0].Typ == itemAt {
						it.Next() // consume '@'
						it.Next() // move forward
						if child.Langs, err = parseLanguageList(it); err != nil {
							return err
						}
					}
					child.Attr = attr
					child.IsInternal = false
				} else {
					if it.Item().Val != value {
						return x.Errorf("Only variables allowed in aggregate functions. Got: %v",
							it.Item().Val)
					}
					count, err := parseVarList(it, child)
					if err != nil {
						return err
					}
					if count != 1 {
						x.Errorf("Expected one variable inside val() of aggregator but got %v", count)
					}
					child.NeedsVar[len(child.NeedsVar)-1].Typ = VALUE_VAR
				}
				child.Func = &Function{
					Name:     valLower,
					NeedsVar: child.NeedsVar,
				}
				it.Next() // Skip the closing ')'
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			} else if isMathBlock(valLower) {
				if varName == "" && alias == "" {
					return x.Errorf("Function math should be used with a variable or have an alias")
				}
				mathTree, again, err := parseMathFunc(it, false)
				if err != nil {
					return err
				}
				if again {
					return x.Errorf("Comma encountered in math() at unexpected place.")
				}
				child := &GraphQuery{
					Attr:       val,
					Alias:      alias,
					Args:       make(map[string]string),
					Var:        varName,
					MathExp:    mathTree,
					IsInternal: true,
				}
				// TODO - See that if we can instead initialize this at the top.
				varName, alias = "", ""
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			} else if isExpandFunc(valLower) {
				if varName != "" {
					return x.Errorf("expand() cannot be used with a variable", val)
				}
				if alias != "" {
					return x.Errorf("expand() cannot have an alias")
				}
				it.Next() // Consume the '('
				if it.Item().Typ != itemLeftRound {
					return x.Errorf("Invalid use of expand()")
				}
				it.Next()
				item := it.Item()
				child := &GraphQuery{
					Attr:       val,
					Args:       make(map[string]string),
					IsInternal: true,
				}
				if item.Val == value {
					count, err := parseVarList(it, child)
					if err != nil {
						return err
					}
					if count != 1 {
						return x.Errorf("Invalid use of expand(). Exactly one variable expected.")
					}
					child.NeedsVar[len(child.NeedsVar)-1].Typ = LIST_VAR
					child.Expand = child.NeedsVar[len(child.NeedsVar)-1].Name
				} else if item.Val == "_all_" {
					child.Expand = "_all_"
				} else {
					return x.Errorf("Invalid argument %v in expand()", item.Val)
				}
				it.Next() // Consume ')'
				gq.Children = append(gq.Children, child)
				// Note: curp is not set to nil. So it can have children, filters, etc.
				curp = child
				continue
			} else if valLower == "count" {
				if count != notSeen {
					return x.Errorf("Invalid mention of function count")
				}
				count = seen
				it.Next()
				item = it.Item()
				if item.Typ != itemLeftRound {
					it.Prev()
					count = notSeen
					goto Fall
				}

				peekIt, err := it.Peek(2)
				if err != nil {
					return err
				}
				if peekIt[0].Typ == itemRightRound {
					return x.Errorf("Cannot use count(), please use count(uid)")
				} else if peekIt[0].Val == uid && peekIt[1].Typ == itemRightRound {
					if gq.IsGroupby {
						// count(uid) case which occurs inside @groupby
						val = uid
						// Skip uid)
						it.Next()
						it.Next()
						goto Fall
					}

					if varName != "" {
						return x.Errorf("Cannot assign variable to count()")
					}
					count = notSeen
					gq.UidCount = true
					if alias != "" {
						gq.UidCountAlias = alias
					}
					it.Next()
					it.Next()
				}
				continue
			} else if valLower == value {
				if varName != "" {
					return x.Errorf("Cannot assign a variable to val()")
				}
				if count == seen {
					return x.Errorf("count of a variable is not allowed")
				}
				peekIt, err = it.Peek(1)
				if err != nil {
					return err
				}
				if peekIt[0].Typ != itemLeftRound {
					goto Fall
				}

				child := &GraphQuery{
					Attr:       val,
					Args:       make(map[string]string),
					IsInternal: true,
					Alias:      alias,
				}
				alias = ""
				count, err := parseVarList(it, child)
				if err != nil {
					return err
				}
				if count != 1 {
					return x.Errorf("Invalid use of val(). Exactly one variable expected.")
				}
				// Only value vars can be retrieved.
				child.NeedsVar[len(child.NeedsVar)-1].Typ = VALUE_VAR
				gq.Children = append(gq.Children, child)
				curp = nil
				continue
			} else if valLower == uid {
				if count == seen {
					return x.Errorf("count of a variable is not allowed")
				}
				peekIt, err = it.Peek(1)
				if err != nil {
					return err
				}
				if peekIt[0].Typ != itemLeftRound {
					goto Fall
				}
				return x.Errorf("Cannot do uid() of a variable")
			}
		Fall:
			if count == seenWithPred {
				return x.Errorf("Multiple predicates not allowed in single count.")
			}
			child := &GraphQuery{
				Args:    make(map[string]string),
				Attr:    val,
				IsCount: count == seen,
				Var:     varName,
				Alias:   alias,
			}

			if gq.IsCount {
				return x.Errorf("Cannot have children attributes when asking for count.")
			}
			gq.Children = append(gq.Children, child)
			varName, alias = "", ""
			curp = child
			if count == seen {
				count = seenWithPred
			}
		case itemLeftCurl:
			if curp == nil {
				return x.Errorf("Query syntax invalid.")
			}
			if len(curp.Langs) > 0 {
				return x.Errorf("Cannot have children for attr: %s with lang tags: %v", curp.Attr,
					curp.Langs)
			}
			if err := godeep(it, curp); err != nil {
				return err
			}
		case itemLeftRound:
			if curp == nil {
				return x.Errorf("Query syntax invalid.")
			}
			if curp.Attr == "" {
				return x.Errorf("Predicate name cannot be empty.")
			}
			args, err := parseArguments(it, curp)
			if err != nil {
				return err
			}
			// Stores args in GraphQuery, will be used later while retrieving results.
			order := make(map[string]bool)
			for _, p := range args {
				if !validKey(p.Key) {
					return x.Errorf("Got invalid keyword: %s", p.Key)
				}
				if _, ok := curp.Args[p.Key]; ok {
					return x.Errorf("Got repeated key %q at level %q", p.Key, curp.Attr)
				}
				if p.Val == "" {
					return x.Errorf("Got empty argument")
				}
				if p.Key == "orderasc" || p.Key == "orderdesc" {
					if order[p.Val] {
						return x.Errorf("Sorting by an attribute: [%s] can only be done once", p.Val)
					}
					attr, langs := attrAndLang(p.Val)
					curp.Order = append(curp.Order, &intern.Order{attr, p.Key == "orderdesc", langs})
					order[p.Val] = true
					continue
				}

				curp.Args[p.Key] = p.Val
			}
		case itemAt:
			err := parseDirective(it, curp)
			if err != nil {
				return err
			}

		case itemRightRound:
			if count != seenWithPred {
				return x.Errorf("Invalid mention of brackets")
			}
			count = notSeen
		}
	}
	return nil
}

func isAggregator(fname string) bool {
	return fname == "min" || fname == "max" || fname == "sum" || fname == "avg"
}

func isExpandFunc(name string) bool {
	return name == "expand"
}

func isMathBlock(name string) bool {
	return name == "math"
}

func isGeoFunc(name string) bool {
	return name == "near" || name == "contains" || name == "within" || name == "intersects"
}

func isInequalityFn(name string) bool {
	switch name {
	case "eq", "le", "ge", "gt", "lt":
		return true
	}
	return false
}

// Name can have dashes or alphanumeric characters. Lexer lexes them as separate items.
// We put it back together here.
func collectName(it *lex.ItemIterator, val string) string {
	var dashOrName bool // false if expecting dash, true if expecting name
	for {
		items, err := it.Peek(1)
		if err == nil && ((items[0].Typ == itemName && dashOrName) ||
			(items[0].Val == "-" && !dashOrName)) {
			it.Next()
			val += it.Item().Val
			dashOrName = !dashOrName
		} else {
			break
		}
	}
	return val
}

// Steps the parser.
func tryParseItemType(it *lex.ItemIterator, typ lex.ItemType) (lex.Item, bool) {
	item, ok := it.PeekOne()
	if !ok || item.Typ != typ {
		return lex.Item{}, false
	}
	it.Next()
	return item, true
}

func trySkipItemVal(it *lex.ItemIterator, val string) bool {
	item, ok := it.PeekOne()
	if !ok || item.Val != val {
		return false
	}
	it.Next()
	return true
}

func trySkipItemTyp(it *lex.ItemIterator, typ lex.ItemType) bool {
	item, ok := it.PeekOne()
	if !ok || item.Typ != typ {
		return false
	}
	it.Next()
	return true
}
