package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/introspection"
	"github.com/vektah/gqlparser/ast"
)

// Introspect performs an introspection query given an operation (contains the query) and a schema.
func Introspect(ctx context.Context, o Operation,
	s Schema) (json.RawMessage, error) {
	sch, ok := s.(*schema)
	if !ok {
		return nil, errors.New("Couldn't convert schema to internal type")
	}

	op, ok := o.(*operation)
	if !ok {
		return nil, errors.New("Couldn't convert operation to internal type")
	}

	// TODO - Fill in graphql variables here instead of an empty map.
	reqCtx := &RequestContext{
		RawQuery:  op.query,
		Variables: map[string]interface{}{},
		Doc:       op.doc,
	}
	ec := executionContext{reqCtx, sch.schema}
	// TODO - This might not always be correct, get the correct selection set here.
	data := ec.handleQuery(ctx, op.op.SelectionSet[0])

	return data, nil
}

type RequestContext struct {
	RawQuery  string
	Variables map[string]interface{}
	Doc       *ast.QueryDocument
}

type executionContext struct {
	*RequestContext
	*ast.Schema
}

func (ec *executionContext) queryType(ctx context.Context,
	w *bytes.Buffer, field graphql.CollectedField) {
	args := field.ArgumentMap(ec.Variables)
	// TODO - What happens if args is nil?
	name := args["name"].(string)
	res := introspection.WrapTypeFromDef(ec.Schema, ec.Schema.Types[name])
	ec.marshalType(ctx, w, field.Selections, res)
}

func (ec *executionContext) querySchema(ctx context.Context,
	w *bytes.Buffer, field graphql.CollectedField) {
	res := introspection.WrapSchema(ec.Schema)
	if res == nil {
		return
	}
	ec.handleSchema(ctx, w, field.Selections, res)
}

func (ec *executionContext) handleTypeFields(ctx context.Context, w *bytes.Buffer,
	field graphql.CollectedField, obj *introspection.Type) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.Fields(args["includeDeprecated"].(bool))
	ec.marshalIntrospectionFieldSlice(ctx, w, field.Selections, res)
}

func (ec *executionContext) handleTypeEnumValues(ctx context.Context, w *bytes.Buffer,
	field graphql.CollectedField,
	obj *introspection.Type) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.EnumValues(args["includeDeprecated"].(bool))
	if res == nil {
		// TODO - Verify we handle types that can/cannot be null properly. Also add test cases for
		// them.
		return
	}
	ec.marshalOptionalEnumValueSlice(ctx, w, field.Selections, res)
}

func writeKey(w *bytes.Buffer, k string) {
	w.WriteRune('"')
	w.WriteString(k)
	w.WriteRune('"')
	w.WriteRune(':')
}

func writeBoolValue(w *bytes.Buffer, val bool) {
	if val {
		w.WriteString("true")
	} else {
		w.WriteString("false")
	}
}

func collectFields(reqCtx *RequestContext, selSet ast.SelectionSet, satisfies []string) []graphql.CollectedField {
	rctx := &graphql.RequestContext{
		RawQuery:  reqCtx.RawQuery,
		Variables: reqCtx.Variables,
		Doc:       reqCtx.Doc,
	}
	return graphql.CollectFields(rctx, selSet, satisfies)
}

func (ec *executionContext) handleQuery(ctx context.Context,
	sel ast.Selection) []byte {
	fields := collectFields(ec.RequestContext, ast.SelectionSet{sel}, []string{"Query"})
	w := new(bytes.Buffer)
	w.WriteRune('{')

	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		// TODO - Make sure alias has the key name when there is no alias.
		writeKey(w, field.Alias)

		switch field.Name {
		// TODO - Add tests for __typename.
		case "__typename":
			w.WriteString(`"Query"`)
		case "__type":
			ec.queryType(ctx, w, field)
		case "__schema":
			ec.querySchema(ctx, w, field)
		default:
		}
	}
	w.WriteRune('}')
	return w.Bytes()
}

func (ec *executionContext) handleDirective(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Directive) {
	fields := collectFields(ec.RequestContext, sel, []string{"__Directive"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Alias)
		switch field.Name {
		case "__typename":
			w.WriteString(`"__Directive"`)
		case "name":
			writeStringValue(w, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
		case "locations":
			ec.marshalDirectionLocationSlice(ctx, w, field.Selections,
				obj.Locations)
		case "args":
			ec.marshalInputValueSlice(ctx, w, field.Selections, obj.Args)
		default:
		}
	}
	w.WriteRune('}')
	return
}

func (ec *executionContext) handleEnumValue(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.EnumValue) {
	fields := collectFields(ec.RequestContext, sel, []string{"__EnumValue"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			writeStringValue(w, "__EnumValue")
		case "name":
			writeStringValue(w, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
		case "isDeprecated":
			writeBoolValue(w, obj.IsDeprecated())
		case "deprecationReason":
			writeOptionalStringValue(w, obj.DeprecationReason())
		default:
		}
	}
	w.WriteRune('}')
	return
}

func (ec *executionContext) handleField(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Field) {
	fields := collectFields(ec.RequestContext, sel, []string{"__Field"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			writeStringValue(w, "__Field")
		case "name":
			writeStringValue(w, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
		case "args":
			ec.marshalInputValueSlice(ctx, w, field.Selections, obj.Args)
		case "type":
			ec.marshalIntrospectionType(ctx, w, field.Selections, obj.Type)
		case "isDeprecated":
			writeBoolValue(w, obj.IsDeprecated())
		case "deprecationReason":
			writeOptionalStringValue(w, obj.DeprecationReason())
		default:
		}
	}
	w.WriteRune('}')
	return
}

func (ec *executionContext) handleInputValue(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.InputValue) {
	fields := collectFields(ec.RequestContext, sel, []string{"__InputValue"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			writeStringValue(w, "__InputValue")
		case "name":
			writeStringValue(w, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
		case "type":
			ec.marshalIntrospectionType(ctx, w, field.Selections, obj.Type)
		case "defaultValue":
			writeOptionalStringValue(w, obj.DefaultValue)
		default:
		}
	}
	w.WriteRune('}')
}

func (ec *executionContext) handleSchema(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Schema) {
	fields := collectFields(ec.RequestContext, sel, []string{"__Schema"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			writeStringValue(w, "__Schema")
		case "types":
			ec.marshalIntrospectionTypeSlice(ctx, w, field.Selections, obj.Types())
		case "queryType":
			ec.marshalIntrospectionType(ctx, w, field.Selections, obj.QueryType())
		case "mutationType":
			ec.marshalType(ctx, w, field.Selections, obj.MutationType())
		case "subscriptionType":
			ec.marshalType(ctx, w, field.Selections, obj.SubscriptionType())
		case "directives":
			ec.marshalDirectiveSlice(ctx, w, field.Selections, obj.Directives())
		default:
		}
	}
	w.WriteRune('}')
}

func writeStringValue(w *bytes.Buffer, val string) {
	w.WriteString(strconv.Quote(val))
}

func writeOptionalStringValue(w *bytes.Buffer, val *string) {
	if val == nil {
		w.WriteString("null")
	} else {
		writeStringValue(w, *val)
	}
}

func writeStringSlice(w *bytes.Buffer, vals []string) {
	w.WriteRune('[')
	for i, v := range vals {
		if i != 0 {
			w.WriteRune(',')
		}
		w.WriteRune('"')
		w.WriteString(v)
		w.WriteRune('"')
	}
	w.WriteRune(']')
	w.WriteRune('"')
}

func (ec *executionContext) handleType(ctx context.Context, w *bytes.Buffer,
	sel ast.SelectionSet, obj *introspection.Type) {
	fields := collectFields(ec.RequestContext, sel, []string{"__Type"})

	w.WriteRune('{')
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Alias)
		switch field.Name {
		case "__typename":
			w.WriteString(`"__Type`)
		case "kind":
			writeStringValue(w, obj.Kind())
		case "name":
			writeOptionalStringValue(w, obj.Name())
		case "description":
			writeStringValue(w, obj.Description())
		case "fields":
			ec.handleTypeFields(ctx, w, field, obj)
		case "interfaces":
			ec.marshalOptionalItypeSlice(ctx, w, field.Selections, obj.Interfaces())
		case "possibleTypes":
			ec.marshalOptionalItypeSlice(ctx, w, field.Selections,
				obj.PossibleTypes())
		case "enumValues":
			ec.handleTypeEnumValues(ctx, w, field, obj)
		case "inputFields":
			ec.marshalOptionalInputValueSlice(ctx, w, field.Selections,
				obj.InputFields())
		case "ofType":
			ec.marshalType(ctx, w, field.Selections, obj.OfType())
		default:
		}
	}
	w.WriteRune('}')
}

func (ec *executionContext) marshalDirectiveSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Directive) {
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleDirective(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
	return
}

func (ec *executionContext) marshalDirectionLocationSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []string) {
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		writeStringValue(w, v[i])
	}
	w.WriteRune(']')
}
func (ec *executionContext) marshalInputValueSlice(ctx context.Context, w *bytes.Buffer,
	sel ast.SelectionSet, v []introspection.InputValue) {
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleInputValue(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalIntrospectionTypeSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Type) {
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleType(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalIntrospectionType(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v *introspection.Type) {
	if v == nil {
		// if !ec.HasError(graphql.GetResolverContext(ctx)) {
		// 	ec.Errorf(ctx, "must not be null")
		// }
		// return graphql.Null
	}
	ec.handleType(ctx, w, sel, v)
}

func (ec *executionContext) marshalOptionalEnumValueSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.EnumValue) {
	// if v == nil {
	// 	return graphql.Null
	// }
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleEnumValue(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalIntrospectionFieldSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.Field) {
	// if v == nil {
	// 	return graphql.Null
	// }
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleField(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalOptionalInputValueSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.InputValue) {
	if v == nil {
		w.WriteString(`null`)
		return
		// return graphql.Null
	}
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleInputValue(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalOptionalItypeSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Type) {
	if v == nil {
		w.WriteString("null")
		return
	}

	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ec.handleType(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
}

func (ec *executionContext) marshalType(ctx context.Context, w *bytes.Buffer,
	sel ast.SelectionSet, v *introspection.Type) {
	if v == nil {
		w.WriteString(`null`)
		return
	}
	ec.handleType(ctx, w, sel, v)
}
