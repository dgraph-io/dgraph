package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlgen/graphql"
	"github.com/dgraph-io/gqlgen/graphql/introspection"
	"github.com/dgraph-io/gqlparser/v2/ast"
)

// Introspection works by walking through the selection set which are part of ast.Operation
// and populating values for different fields. We have a dependency on gqlgen packages because
// a) they define some useful types like introspection.Type, introspection.InputValue,
// introspection.Directive etc.
// b) CollectFields function which can recursively expand fragments and convert them to fields
// and selection sets.
// We might be able to get rid of this dependency in the future as we support fragments in other
// queries or we might get rid of the types defined in wrappers.go and use the types defined in
// gqlgen instead if they make more sense.

// Introspect performs an introspection query given a query that's expected to be either
// __schema or __type.
func Introspect(q Query) (json.RawMessage, error) {
	if q.Name() != "__schema" && q.Name() != "__type" && q.Name() != Typename {
		return nil, errors.New("call to introspect for field that isn't an introspection query " +
			"this indicates bug. Please let us know by filing an issue")
	}

	sch, ok := q.Operation().Schema().(*schema)
	if !ok {
		return nil, errors.New("couldn't convert schema to internal type " +
			"this indicates bug. Please let us know by filing an issue")
	}

	op, ok := q.Operation().(*operation)
	if !ok {
		return nil, errors.New("couldn't convert operation to internal type " +
			"this indicates bug. Please let us know by filing an issue")
	}

	qu, ok := q.(*query)
	if !ok {
		return nil, errors.New("couldn't convert query to internal type " +
			"this indicates bug. Please let us know by filing an issue")
	}

	reqCtx := &requestContext{
		RawQuery:  op.query,
		Variables: op.vars,
		Doc:       op.doc,
	}
	ec := executionContext{reqCtx, sch.schema, new(bytes.Buffer)}
	return ec.handleQuery(qu.sel), nil
}

type requestContext struct {
	RawQuery  string
	Variables map[string]interface{}
	Doc       *ast.QueryDocument
}

type executionContext struct {
	*requestContext
	*ast.Schema
	b *bytes.Buffer // we build the JSON response and write it to b.
}

func (ec *executionContext) writeKey(k string) {
	x.Check2(ec.b.WriteRune('"'))
	x.Check2(ec.b.WriteString(k))
	x.Check2(ec.b.WriteRune('"'))
	x.Check2(ec.b.WriteRune(':'))
}

func (ec *executionContext) writeBoolValue(val bool) {
	if val {
		x.Check2(ec.b.WriteString("true"))
	} else {
		x.Check2(ec.b.WriteString("false"))
	}
}

func (ec *executionContext) writeStringValue(val string) {
	x.Check2(ec.b.WriteString(strconv.Quote(val)))
}

func (ec *executionContext) writeOptionalStringValue(val *string) {
	if val == nil {
		x.Check2(ec.b.Write(JsonNull))
	} else {
		ec.writeStringValue(*val)
	}
}

func (ec *executionContext) writeStringSlice(v []string) {
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeStringValue(v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

// collectFields is our wrapper around graphql.CollectFields which is able to build a tree (after
// expanding fragments) represented by []graphql.CollectorField. It requires passing the
// graphql.OperationContext to work correctly.
func collectFields(reqCtx *requestContext, selSet ast.SelectionSet,
	satisfies []string) []graphql.CollectedField {
	ctx := &graphql.OperationContext{
		RawQuery:  reqCtx.RawQuery,
		Variables: reqCtx.Variables,
		Doc:       reqCtx.Doc,
	}
	return graphql.CollectFields(ctx, selSet, satisfies)
}

func (ec *executionContext) queryType(field graphql.CollectedField) {
	args := field.ArgumentMap(ec.Variables)
	name := args["name"].(string)
	res := introspection.WrapTypeFromDef(ec.Schema, ec.Schema.Types[name])
	ec.marshalType(field.Selections, res)
}

func (ec *executionContext) querySchema(field graphql.CollectedField) {
	res := introspection.WrapSchema(ec.Schema)
	if res == nil {
		return
	}
	ec.handleSchema(field.Selections, res)
}

func (ec *executionContext) handleTypeFields(field graphql.CollectedField,
	obj *introspection.Type) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.Fields(args["includeDeprecated"].(bool))
	ec.marshalIntrospectionFieldSlice(field.Selections, res)
}

func (ec *executionContext) handleTypeEnumValues(field graphql.CollectedField,
	obj *introspection.Type) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.EnumValues(args["includeDeprecated"].(bool))
	if res == nil {
		// TODO - Verify we handle types that can/cannot be null properly. Also add test cases for
		// them.
		return
	}
	ec.marshalOptionalEnumValueSlice(field.Selections, res)
}

func (ec *executionContext) handleQuery(sel ast.Selection) []byte {
	fields := collectFields(ec.requestContext, ast.SelectionSet{sel}, []string{"Query"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			x.Check2(ec.b.WriteString(`"Query"`))
		case "__type":
			ec.queryType(field)
		case "__schema":
			ec.querySchema(field)
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
	return ec.b.Bytes()
}

func (ec *executionContext) handleDirective(sel ast.SelectionSet, obj *introspection.Directive) {
	fields := collectFields(ec.requestContext, sel, []string{"__Directive"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			x.Check2(ec.b.WriteString(`"__Directive"`))
		case "name":
			ec.writeStringValue(obj.Name)
		case "description":
			ec.writeStringValue(obj.Description)
		case "locations":
			ec.writeStringSlice(obj.Locations)
		case "args":
			ec.marshalInputValueSlice(field.Selections, obj.Args)
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) handleEnumValue(sel ast.SelectionSet, obj *introspection.EnumValue) {
	fields := collectFields(ec.requestContext, sel, []string{"__EnumValue"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			ec.writeStringValue("__EnumValue")
		case "name":
			ec.writeStringValue(obj.Name)
		case "description":
			ec.writeStringValue(obj.Description)
		case "isDeprecated":
			ec.writeBoolValue(obj.IsDeprecated())
		case "deprecationReason":
			ec.writeOptionalStringValue(obj.DeprecationReason())
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) handleField(sel ast.SelectionSet, obj *introspection.Field) {
	fields := collectFields(ec.requestContext, sel, []string{"__Field"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			ec.writeStringValue("__Field")
		case "name":
			ec.writeStringValue(obj.Name)
		case "description":
			ec.writeStringValue(obj.Description)
		case "args":
			ec.marshalInputValueSlice(field.Selections, obj.Args)
		case "type":
			ec.marshalIntrospectionType(field.Selections, obj.Type)
		case "isDeprecated":
			ec.writeBoolValue(obj.IsDeprecated())
		case "deprecationReason":
			ec.writeOptionalStringValue(obj.DeprecationReason())
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) handleInputValue(sel ast.SelectionSet, obj *introspection.InputValue) {
	fields := collectFields(ec.requestContext, sel, []string{"__InputValue"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			ec.writeStringValue("__InputValue")
		case "name":
			ec.writeStringValue(obj.Name)
		case "description":
			ec.writeStringValue(obj.Description)
		case "type":
			ec.marshalIntrospectionType(field.Selections, obj.Type)
		case "defaultValue":
			ec.writeOptionalStringValue(obj.DefaultValue)
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) handleSchema(sel ast.SelectionSet, obj *introspection.Schema) {
	fields := collectFields(ec.requestContext, sel, []string{"__Schema"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			ec.writeStringValue("__Schema")
		case "types":
			// obj.Types() does not return all the types in the schema, it ignores the ones
			// named like __TypeName, so using getAllTypes()
			ec.marshalIntrospectionTypeSlice(field.Selections, getAllTypes(ec.Schema))
		case "queryType":
			ec.marshalIntrospectionType(field.Selections, obj.QueryType())
		case "mutationType":
			ec.marshalType(field.Selections, obj.MutationType())
		case "subscriptionType":
			ec.marshalType(field.Selections, obj.SubscriptionType())
		case "directives":
			ec.marshalDirectiveSlice(field.Selections, obj.Directives())
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) handleType(sel ast.SelectionSet, obj *introspection.Type) {
	fields := collectFields(ec.requestContext, sel, []string{"__Type"})

	x.Check2(ec.b.WriteRune('{'))
	for i, field := range fields {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.writeKey(field.Alias)
		switch field.Name {
		case Typename:
			x.Check2(ec.b.WriteString(`"__Type"`))
		case "kind":
			ec.writeStringValue(obj.Kind())
		case "name":
			ec.writeOptionalStringValue(obj.Name())
		case "description":
			ec.writeStringValue(obj.Description())
		case "fields":
			ec.handleTypeFields(field, obj)
		case "interfaces":
			ec.marshalOptionalItypeSlice(field.Selections, obj.Interfaces())
		case "possibleTypes":
			ec.marshalOptionalItypeSlice(field.Selections, obj.PossibleTypes())
		case "enumValues":
			ec.handleTypeEnumValues(field, obj)
		case "inputFields":
			ec.marshalOptionalInputValueSlice(field.Selections, obj.InputFields())
		case "ofType":
			ec.marshalType(field.Selections, obj.OfType())
		default:
		}
	}
	x.Check2(ec.b.WriteRune('}'))
}

func (ec *executionContext) marshalDirectiveSlice(sel ast.SelectionSet,
	v []introspection.Directive) {
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleDirective(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalInputValueSlice(sel ast.SelectionSet,
	v []introspection.InputValue) {
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleInputValue(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalIntrospectionTypeSlice(sel ast.SelectionSet,
	v []introspection.Type) {
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleType(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalIntrospectionType(sel ast.SelectionSet, v *introspection.Type) {
	if v == nil {
		// TODO - This should be an error as this field is mandatory.
		x.Check2(ec.b.Write(JsonNull))
		return
	}
	ec.handleType(sel, v)
}

func (ec *executionContext) marshalOptionalEnumValueSlice(sel ast.SelectionSet,
	v []introspection.EnumValue) {
	if v == nil {
		x.Check2(ec.b.Write(JsonNull))
		return
	}
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleEnumValue(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalIntrospectionFieldSlice(sel ast.SelectionSet,
	v []introspection.Field) {
	if v == nil {
		x.Check2(ec.b.Write(JsonNull))
		return
	}
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleField(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalOptionalInputValueSlice(sel ast.SelectionSet,
	v []introspection.InputValue) {
	if v == nil {
		x.Check2(ec.b.WriteString(`null`))
		return
	}
	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleInputValue(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalOptionalItypeSlice(sel ast.SelectionSet,
	v []introspection.Type) {
	if v == nil {
		x.Check2(ec.b.Write(JsonNull))
		return
	}

	x.Check2(ec.b.WriteRune('['))
	for i := range v {
		if i != 0 {
			x.Check2(ec.b.WriteRune(','))
		}
		ec.handleType(sel, &v[i])
	}
	x.Check2(ec.b.WriteRune(']'))
}

func (ec *executionContext) marshalType(sel ast.SelectionSet, v *introspection.Type) {
	if v == nil {
		x.Check2(ec.b.Write(JsonNull))
		return
	}
	ec.handleType(sel, v)
}

// Returns all the types associated with the schema, including the ones that are part
// of introspection system (i.e., the type name begins with __ )
func getAllTypes(s *ast.Schema) []introspection.Type {
	types := make([]introspection.Type, 0, len(s.Types))
	for _, typ := range s.Types {
		types = append(types, *introspection.WrapTypeFromDef(s, typ))
	}
	return types
}
