package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	reqCtx := graphql.NewRequestContext(op.doc, op.query, map[string]interface{}{})
	ctx = graphql.WithRequestContext(ctx, reqCtx)

	ec := executionContext{reqCtx, sch.schema}
	data := ec.handleQuery(ctx, op.op.SelectionSet)
	var buf bytes.Buffer
	data.MarshalGQL(&buf)
	d := buf.Bytes()

	return d, nil
}

type executionContext struct {
	*graphql.RequestContext
	*ast.Schema
}

func (ec *executionContext) queryType(ctx context.Context,
	w *bytes.Buffer, field graphql.CollectedField) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	// TODO - What happens if args is nil?
	name := args["name"].(string)
	res := introspection.WrapTypeFromDef(ec.Schema, ec.Schema.Types[name])
	return ec.marshalType(ctx, w, field.Selections, res)
}

func (ec *executionContext) querySchema(ctx context.Context,
	w *bytes.Buffer, field graphql.CollectedField) (ret graphql.Marshaler) {
	res := introspection.WrapSchema(ec.Schema)
	if res == nil {
		return graphql.Null
	}
	return ec.handleSchema(ctx, w, field.Selections, res)
}

func (ec *executionContext) handleTypeFields(ctx context.Context, w *bytes.Buffer,
	field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.Fields(args["includeDeprecated"].(bool))
	return ec.marshalIntrospectionFieldSlice(ctx, w, field.Selections, res)
}

func (ec *executionContext) handleTypeEnumValues(ctx context.Context, w *bytes.Buffer,
	field graphql.CollectedField,
	obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.EnumValues(args["includeDeprecated"].(bool))
	if res == nil {
		// TODO - Verify we handle types that can/cannot be null properly. Also add test cases for
		// them.
		return graphql.Null
	}
	return ec.marshalOptionalEnumValueSlice(ctx, w, field.Selections, res)
}

func writeKey(w *bytes.Buffer, k string) {
	w.WriteRune('"')
	w.WriteString(k)
	w.WriteRune('"')
	w.WriteRune(':')
}

func (ec *executionContext) handleQuery(ctx context.Context,
	sel ast.SelectionSet) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"Query"})
	w := new(bytes.Buffer)
	w.WriteRune('{')

	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		// TODO - Make sure alias has the key name when there is no alias.
		writeKey(w, field.Alias)

		switch field.Name {
		// TODO - Add tests for __typename.
		case "__typename":
			out.Values[i] = graphql.MarshalString("Query")
			w.WriteString(`"Query"`)
		case "__type":
			out.Values[i] = ec.queryType(ctx, w, field)
		case "__schema":
			out.Values[i] = ec.querySchema(ctx, w, field)
		default:
		}
	}
	w.WriteRune('}')
	fmt.Println(w.String())
	out.Dispatch()
	return out
}

func (ec *executionContext) handleDirective(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Directive) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Directive"})

	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			w.WriteString(`"__Directive"`)
			out.Values[i] = graphql.MarshalString("__Directive")
		case "name":
			writeStringValue(w, obj.Name)
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "locations":
			writeStringSlice(w, obj.Locations)
			out.Values[i] = ec.marshalDirectionLocationSlice(ctx, field.Selections,
				obj.Locations)
		case "args":
			out.Values[i] = ec.marshalInputValueSlice(ctx, w, field.Selections, obj.Args)
		default:
		}
	}
	out.Dispatch()
	return out
}

func (ec *executionContext) handleEnumValue(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.EnumValue) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__EnumValue"})

	w.WriteRune('{')
	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__EnumValue")
		case "name":
			writeStringValue(w, obj.Name)
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections, obj.IsDeprecated())
		case "deprecationReason":
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections,
				obj.DeprecationReason())
		default:
		}
	}
	w.WriteRune('}')
	out.Dispatch()
	return out
}

func (ec *executionContext) handleField(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Field) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Field"})

	w.WriteRune('{')
	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Name)
		switch field.Name {
		case "__typename":
			writeStringValue(w, "__Field")
			out.Values[i] = graphql.MarshalString("__Field")
		case "name":
			writeStringValue(w, obj.Name)
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
		case "description":
			writeStringValue(w, obj.Description)
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "args":
			out.Values[i] = ec.marshalInputValueSlice(ctx, w, field.Selections, obj.Args)
		case "type":
			out.Values[i] = ec.marshalIntrospectionType(ctx, w, field.Selections, obj.Type)
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections,
				obj.IsDeprecated())
		case "deprecationReason":
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections,
				obj.DeprecationReason())
		default:
		}
	}
	w.WriteRune('}')
	out.Dispatch()
	return out
}

func (ec *executionContext) handleInputValue(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.InputValue) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__InputValue"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__InputValue")
		case "name":
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "description":
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "type":
			out.Values[i] = ec.marshalIntrospectionType(ctx, w, field.Selections, obj.Type)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "defaultValue":
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.DefaultValue)
		default:
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleSchema(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	obj *introspection.Schema) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Schema"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Schema")
		case "types":
			out.Values[i] = ec.marshalIntrospectionTypeSlice(ctx, w, field.Selections, obj.Types())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "queryType":
			out.Values[i] = ec.marshalIntrospectionType(ctx, w, field.Selections, obj.QueryType())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "mutationType":
			out.Values[i] = ec.marshalType(ctx, w, field.Selections, obj.MutationType())
		case "subscriptionType":
			out.Values[i] = ec.marshalType(ctx, w, field.Selections, obj.SubscriptionType())
		case "directives":
			out.Values[i] = ec.marshalDirectiveSlice(ctx, w, field.Selections, obj.Directives())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		default:
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func writeStringValue(w *bytes.Buffer, val string) {
	w.WriteRune('"')
	w.WriteString(val)
	w.WriteRune('"')
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
	sel ast.SelectionSet, obj *introspection.Type) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Type"})

	w.WriteRune('{')
	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if i != 0 {
			w.WriteRune(',')
		}
		writeKey(w, field.Alias)
		switch field.Name {
		case "__typename":
			w.WriteString(`"__Type`)
			out.Values[i] = graphql.MarshalString("__Type")
		case "kind":
			writeStringValue(w, obj.Kind())
			out.Values[i] = ec.marshalNTypeKind2string(ctx, field.Selections, obj.Kind())
			if out.Values[i] == graphql.Null {
				return graphql.Null
			}
		case "name":
			writeStringValue(w, *(obj.Name()))
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.Name())
		case "description":
			writeStringValue(w, obj.Description())
			out.Values[i] = graphql.MarshalString(obj.Description())
		case "fields":
			out.Values[i] = ec.handleTypeFields(ctx, w, field, obj)
		case "interfaces":
			out.Values[i] = ec.marshalOptionalItypeSlice(ctx, w, field.Selections, obj.Interfaces())
		case "possibleTypes":
			out.Values[i] = ec.marshalOptionalItypeSlice(ctx, w, field.Selections,
				obj.PossibleTypes())
		case "enumValues":
			out.Values[i] = ec.handleTypeEnumValues(ctx, w, field, obj)
		case "inputFields":
			out.Values[i] = ec.marshalOptionalInputValueSlice(ctx, w, field.Selections,
				obj.InputFields())
		case "ofType":
			out.Values[i] = ec.marshalType(ctx, w, field.Selections, obj.OfType())
		default:
		}
	}
	w.WriteRune('}')
	out.Dispatch()
	return out
}

func (ec *executionContext) marshalNBoolean2bool(ctx context.Context, sel ast.SelectionSet,
	v bool) graphql.Marshaler {
	res := graphql.MarshalBoolean(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalNString2string(ctx context.Context, sel ast.SelectionSet,
	v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		// TODO - Check that resolver gets this information.
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalDirectiveSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Directive) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleDirective(ctx, w, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalDirectionLocation(ctx context.Context,
	sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalDirectionLocationSlice(ctx context.Context,
	sel ast.SelectionSet, v []string) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalDirectionLocation(ctx, sel, v[i])
	}
	return ret
}
func (ec *executionContext) marshalInputValueSlice(ctx context.Context, w *bytes.Buffer,
	sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleInputValue(ctx, w, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalIntrospectionTypeSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Type) graphql.Marshaler {
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.handleType(ctx, w, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalIntrospectionType(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v *introspection.Type) graphql.Marshaler {
	if v == nil {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
		return graphql.Null
	}
	return ec.handleType(ctx, w, sel, v)
}

func (ec *executionContext) marshalNTypeKind2string(ctx context.Context, sel ast.SelectionSet,
	v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalOString2string(ctx context.Context, sel ast.SelectionSet,
	v *string) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return graphql.MarshalString(*v)
}

func (ec *executionContext) marshalOptionalEnumValueSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.EnumValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ret[i] = ec.handleEnumValue(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
	return ret
}

func (ec *executionContext) marshalIntrospectionFieldSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.Field) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	w.WriteRune('[')
	for i := range v {
		if i != 0 {
			w.WriteRune(',')
		}
		ret[i] = ec.handleField(ctx, w, sel, &v[i])
	}
	w.WriteRune(']')
	return ret
}

func (ec *executionContext) marshalOptionalInputValueSlice(ctx context.Context,
	w *bytes.Buffer, sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleInputValue(ctx, w, sel, &v[i])

	}
	return ret
}

func (ec *executionContext) marshalOptionalItypeSlice(ctx context.Context, w *bytes.Buffer, sel ast.SelectionSet,
	v []introspection.Type) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.handleType(ctx, w, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalType(ctx context.Context, w *bytes.Buffer,
	sel ast.SelectionSet, v *introspection.Type) graphql.Marshaler {
	if v == nil {
		w.WriteString(`"null"`)
		return graphql.Null
	}
	return ec.handleType(ctx, w, sel, v)
}
