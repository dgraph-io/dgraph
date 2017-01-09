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
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

// GraphQuery stores the parsed Query in a tree format. This gets converted to
// internally used query.SubGraph before processing the query.
type GraphQuery struct {
	UID   uint64
	XID   string
	Attr  string
	Alias string
	Func  *Function

	Args     map[string]string
	Children []*GraphQuery
	Filter   *FilterTree

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

type varInfo struct {
	Value string
	Type  string
}

// varMap is a map with key as variable name.
type varMap map[string]varInfo

// FilterTree is the result of parsing the filter directive.
type FilterTree struct {
	Op    string
	Func  *Function
	Child []*FilterTree
}

// Function holds the information about gql functions.
type Function struct {
	Attr string
	Name string   // Specifies the name of the function.
	Args []string // Contains the arguments of the function.
}

// filterOpPrecedence is a map from filterOp (a string) to its precedence.
var filterOpPrecedence map[string]int

func init() {
	filterOpPrecedence = map[string]int{
		"&": 2,
		"|": 1,
	}
}

// DebugPrint is useful for debugging.
func (gq *GraphQuery) DebugPrint(prefix string) {
	x.Printf("%s[%x %q %q->%q]\n", prefix, gq.UID, gq.XID, gq.Attr, gq.Alias)
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

func checkValidity(vm varMap) error {
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

// Parse initializes and runs the lexer. It also constructs the GraphQuery subgraph
// from the lexed items.
func Parse(input string) (gq *GraphQuery, mu *Mutation, rerr error) {
	query, vmap, err := parseQueryWithVariables(input)
	if err != nil {
		return nil, nil, err
	}

	l := lex.NewLexer(query)
	l.Run(lexText)

	it := l.NewIterator()
	fmap := make(fragmentMap)
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemError:
			return nil, nil, x.Errorf(item.Val)
		case itemText:
			continue

		case itemOpType:
			if item.Val == "mutation" {
				if mu != nil {
					return nil, nil, x.Errorf("Only one mutation block allowed.")
				}
				if mu, rerr = getMutation(it); rerr != nil {
					return nil, nil, rerr
				}
			} else if item.Val == "fragment" {
				// TODO(jchiu0): This is to be done in ParseSchema once it is ready.
				fnode, rerr := getFragment(it)
				if rerr != nil {
					return nil, nil, rerr
				}
				fmap[fnode.Name] = fnode
			} else if item.Val == "query" {
				if gq, rerr = getVariablesAndQuery(it, vmap); rerr != nil {
					return nil, nil, rerr
				}
			}
		case itemLeftCurl:
			if gq, rerr = getQuery(it); rerr != nil {
				return nil, nil, rerr
			}
		}
	}

	if gq != nil {
		// Try expanding fragments using fragment map.
		if err := gq.expandFragments(fmap); err != nil {
			return nil, nil, err
		}

		// Substitute all variables with corresponding values
		if err := substituteVariables(gq, vmap); err != nil {
			return nil, nil, err
		}
	}

	return gq, mu, nil
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

			if rerr = checkValidity(vmap); rerr != nil {
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
	it.Next()
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
	for it.Next() {
		var varName string
		// Get variable name.
		item := it.Item()
		if item.Typ == itemDollar {
			it.Next()
			item = it.Item()
			if item.Typ == itemName {
				varName = fmt.Sprintf("$%s", item.Val)
			} else {
				return x.Errorf("Expecting a variable name. Got: %v", item)
			}
		} else if item.Typ == itemRightRound {
			break
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
	}
	return nil
}

// parseArguments parses the arguments part of the GraphQL query root.
func parseArguments(it *lex.ItemIterator) (result []pair, rerr error) {
	for it.Next() {
		var p pair
		// Get key.
		item := it.Item()
		if item.Typ == itemName {
			p.Key = item.Val

		} else if item.Typ == itemRightRound {
			break

		} else {
			return result, x.Errorf("Expecting argument name. Got: %v", item)
		}

		it.Next()
		item = it.Item()
		if item.Typ != itemColon {
			return result, x.Errorf("Expecting a collon. Got: %v", item)
		}

		// Get value.
		it.Next()
		item = it.Item()
		var val string
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
		_, err := buf.WriteRune('(')
		x.Check(err)
		_, err = buf.WriteString(t.Func.Name)
		x.Check(err)

		if len(t.Func.Attr) > 0 {
			args := make([]string, len(t.Func.Args)+1)
			args[0] = t.Func.Attr
			copy(args[1:], t.Func.Args)

			for _, arg := range args {
				_, err = buf.WriteString(" \"")
				x.Check(err)
				_, err = buf.WriteString(arg)
				x.Check(err)
				_, err := buf.WriteRune('"')
				x.Check(err)
			}
		}
		_, err = buf.WriteRune(')')
		x.Check(err)
		return
	}
	// Non-leaf node.
	_, err := buf.WriteRune('(')
	x.Check(err)
	switch t.Op {
	case "&":
		_, err = buf.WriteString("AND")
	case "|":
		_, err = buf.WriteString("OR")
	case "(":
		_, err = buf.WriteString("(")
	default:
		err = x.Errorf("Unknown operator: %q", t.Op)
	}
	x.Check(err)

	for _, c := range t.Child {
		_, err = buf.WriteRune(' ')
		x.Check(err)
		c.stringHelper(buf)
	}
	_, err = buf.WriteRune(')')
	x.Check(err)
}

type filterTreeStack struct{ a []*FilterTree }

func (s *filterTreeStack) empty() bool        { return len(s.a) == 0 }
func (s *filterTreeStack) size() int          { return len(s.a) }
func (s *filterTreeStack) push(t *FilterTree) { s.a = append(s.a, t) }

func (s *filterTreeStack) pop() *FilterTree {
	x.AssertTruef(!s.empty(), "Trying to pop empty stack")
	last := s.a[len(s.a)-1]
	s.a = s.a[:len(s.a)-1]
	return last
}

func (s *filterTreeStack) peek() *FilterTree {
	x.AssertTruef(!s.empty(), "Trying to peek empty stack")
	return s.a[len(s.a)-1]
}

func evalStack(opStack, valueStack *filterTreeStack) {
	topOp := opStack.pop()
	topVal1 := valueStack.pop()
	topVal2 := valueStack.pop()
	topOp.Child = []*FilterTree{topVal2, topVal1}
	valueStack.push(topOp)
}

func parseFunction(it *lex.ItemIterator) (*Function, error) {
	var g *Function
	for it.Next() {
		item := it.Item()
		if item.Typ == itemName { // Value.
			g = &Function{Name: item.Val}
			it.Next()
			itemInFunc := it.Item()
			if itemInFunc.Typ != itemLeftRound {
				return nil, x.Errorf("Expected ( after func name [%s]", g.Name)
			}
			for it.Next() {
				itemInFunc := it.Item()
				if itemInFunc.Typ == itemRightRound {
					break
				} else if itemInFunc.Typ != itemName {
					return nil, x.Errorf("Expected arg after func [%s], but got item %v",
						g.Name, itemInFunc)
				}
				it := strings.Trim(itemInFunc.Val, "\" \t")
				if it == "" {
					return nil, x.Errorf("Empty argument received")
				}
				if len(g.Attr) == 0 {
					g.Attr = it
				} else {
					g.Args = append(g.Args, it)
				}
			}
		} else if item.Typ == itemRightRound {
			break
		} else {
			return nil, x.Errorf("Expected a function but got %q", item.Val)
		}
	}
	return g, nil
}

// parseFilter parses the filter directive to produce a QueryFilter / parse tree.
func parseFilter(it *lex.ItemIterator) (*FilterTree, error) {
	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected ( after filter directive")
	}

	opStack := new(filterTreeStack)
	opStack.push(&FilterTree{Op: "("}) // Push ( onto operator stack.
	valueStack := new(filterTreeStack)

	for it.Next() {
		item := it.Item()
		if item.Typ == itemName { // Value.
			f := &Function{}
			leaf := &FilterTree{Func: f}
			f.Name = item.Val
			it.Next()
			itemInFunc := it.Item()
			if itemInFunc.Typ != itemLeftRound {
				return nil, x.Errorf("Expected ( after func name [%s]", leaf.Func.Name)
			}
			var terminated bool
			for it.Next() {
				itemInFunc := it.Item()
				if itemInFunc.Typ == itemRightRound {
					terminated = true
					break
				} else if itemInFunc.Typ != itemName {
					return nil, x.Errorf("Expected arg after func [%s], but got item %v",
						leaf.Func.Name, itemInFunc)
				}
				it := strings.Trim(itemInFunc.Val, "\" \t")
				if it == "" {
					return nil, x.Errorf("Empty argument received")
				}
				if len(f.Attr) == 0 {
					f.Attr = it
				} else {
					f.Args = append(f.Args, it)
				}
			}
			if !terminated {
				return nil, x.Errorf("Expected ) to terminate func definition")
			}
			valueStack.push(leaf)

		} else if item.Typ == itemLeftRound { // Just push to op stack.
			opStack.push(&FilterTree{Op: "("})

		} else if item.Typ == itemRightRound { // Pop op stack until we see a (.
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Op == "(" {
					break
				}
				evalStack(opStack, valueStack)
			}
			opStack.pop() // Pop away the (.
			if opStack.empty() {
				// The parentheses are balanced out. Let's break.
				break
			}

		} else if item.Typ == itemAnd || item.Typ == itemOr {
			op := "&"
			if item.Typ == itemOr {
				op = "|"
			}
			opPred := filterOpPrecedence[op]
			x.AssertTruef(opPred > 0, "Expected opPred > 0: %d", opPred)
			// Evaluate the stack until we see an operator with strictly lower pred.
			for !opStack.empty() {
				topOp := opStack.peek()
				if filterOpPrecedence[topOp.Op] < opPred {
					break
				}
				evalStack(opStack, valueStack)
			}
			opStack.push(&FilterTree{Op: op}) // Push current operator.
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
	return valueStack.pop(), nil
}

// getRoot gets the root graph query object after parsing the args.
func getRoot(it *lex.ItemIterator) (gq *GraphQuery, rerr error) {
	gq = &GraphQuery{
		Args: make(map[string]string),
	}
	it.Next()
	item := it.Item()
	if item.Typ != itemName {
		return nil, x.Errorf("Expected some name. Got: %v", item)
	}

	gq.Alias = item.Val
	it.Next()
	item = it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected variable start. Got: %v", item)
	}

	// Peeks items to see if its an argument or function.
	peekItems, err := it.Peek(2)
	if err != nil {
		return gq, err
	}
	if peekItems[1].Typ == itemLeftRound {
		// Store the generator function.
		gen, err := parseFunction(it)
		if err != nil {
			return gq, err
		}
		if !schema.IsIndexed(gen.Attr) {
			return nil, x.Errorf(
				"Field %s is not indexed and cannot be used in functions",
				gen.Attr)
		}
		if err != nil {
			return nil, err
		}
		gq.Func = gen
	} else if peekItems[1].Typ == itemColon {
		args, err := parseArguments(it)
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
				return nil, x.Errorf("Expecting _uid_ or _xid_. Got: %+v", p)
			}
		}
	} else {
		return nil, x.Errorf("Unexpected root argument. Got: %v", peekItems)
	}

	return gq, nil
}

// godeep constructs the subgraph from the lexed items and a GraphQuery node.
func godeep(it *lex.ItemIterator, gq *GraphQuery) error {
	curp := gq // Used to track current node, for nesting.
	for it.Next() {
		item := it.Item()
		if item.Typ == lex.ItemError {
			return x.Errorf(item.Val)
		}

		if item.Typ == lex.ItemEOF {
			return nil
		}

		if item.Typ == itemRightCurl {
			return nil
		}

		if item.Typ == itemThreeDots {
			it.Next()
			item = it.Item()
			if item.Typ == itemName {
				// item.Val is expected to start with "..." and to have len >3.
				gq.Children = append(gq.Children, &GraphQuery{fragment: item.Val})
				// Unlike itemName, there is no nesting, so do not change "curp".
			}
		} else if item.Typ == itemName {
			child := &GraphQuery{
				Args: make(map[string]string),
				Attr: item.Val,
			}
			gq.Children = append(gq.Children, child)
			curp = child

		} else if item.Typ == itemColon {
			it.Next()
			item = it.Item()
			if item.Typ != itemName {
				return x.Errorf("Predicate Expected but got: %s", item.Val)
			}
			curp.Alias = curp.Attr
			curp.Attr = item.Val
		} else if item.Typ == itemLeftCurl {
			if err := godeep(it, curp); err != nil {
				return err
			}

		} else if item.Typ == itemLeftRound {
			args, err := parseArguments(it)
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
		} else if item.Typ == itemAt {
			it.Next()
			item := it.Item()
			if item.Typ == itemName {
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
			}
		}
	}
	return nil
}
