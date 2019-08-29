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
	field graphql.CollectedField) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	// TODO - What happens if args is nil?
	name := args["name"].(string)
	res := introspection.WrapTypeFromDef(ec.Schema, ec.Schema.Types[name])
	return ec.marshalType(ctx, field.Selections, res)
}

func (ec *executionContext) querySchema(ctx context.Context,
	field graphql.CollectedField) (ret graphql.Marshaler) {
	res := introspection.WrapSchema(ec.Schema)
	if res == nil {
		return graphql.Null
	}
	return ec.handleSchema(ctx, field.Selections, res)
}

func (ec *executionContext) handleTypeFields(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.Fields(args["includeDeprecated"].(bool))
	return ec.marshalIntrospectionFieldSlice(ctx, field.Selections, res)
}

func (ec *executionContext) handleTypeEnumValues(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.EnumValues(args["includeDeprecated"].(bool))
	if res == nil {
		return graphql.Null
	}
	return ec.marshalOptionalEnumValueSlice(ctx, field.Selections, res)
}

func (ec *executionContext) handleQuery(ctx context.Context,
	sel ast.SelectionSet) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"Query"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("Query")
		case "__type":
			out.Values[i] = ec.queryType(ctx, field)
		case "__schema":
			out.Values[i] = ec.querySchema(ctx, field)
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleDirective(ctx context.Context, sel ast.SelectionSet,
	obj *introspection.Directive) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Directive"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Directive")
		case "name":
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "description":
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "locations":
			out.Values[i] = ec.marshalN__DirectiveLocation2ᚕstring(ctx, field.Selections,
				obj.Locations)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "args":
			out.Values[i] = ec.marshalInputValueSlice(ctx, field.Selections, obj.Args)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleEnumValue(ctx context.Context, sel ast.SelectionSet,
	obj *introspection.EnumValue) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__EnumValue"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__EnumValue")
		case "name":
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "description":
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections, obj.IsDeprecated())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "deprecationReason":
			out.Values[i] = ec.marshalOString2ᚖstring(ctx, field.Selections,
				obj.DeprecationReason())
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleField(ctx context.Context, sel ast.SelectionSet,
	obj *introspection.Field) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Field"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Field")
		case "name":
			out.Values[i] = ec.marshalNString2string(ctx, field.Selections, obj.Name)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "description":
			out.Values[i] = graphql.MarshalString(obj.Description)
		case "args":
			out.Values[i] = ec.marshalInputValueSlice(ctx, field.Selections, obj.Args)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "type":
			out.Values[i] = ec.marshalIntrospectionType(ctx, field.Selections, obj.Type)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections, obj.IsDeprecated())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "deprecationReason":
			out.Values[i] = ec.marshalOString2ᚖstring(ctx, field.Selections,
				obj.DeprecationReason())
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleInputValue(ctx context.Context, sel ast.SelectionSet,
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
			out.Values[i] = ec.marshalIntrospectionType(ctx, field.Selections, obj.Type)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "defaultValue":
			out.Values[i] = ec.marshalOString2ᚖstring(ctx, field.Selections, obj.DefaultValue)
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleSchema(ctx context.Context, sel ast.SelectionSet,
	obj *introspection.Schema) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Schema"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Schema")
		case "types":
			out.Values[i] = ec.marshalIntrospectionTypeSlice(ctx, field.Selections, obj.Types())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "queryType":
			out.Values[i] = ec.marshalIntrospectionType(ctx, field.Selections, obj.QueryType())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "mutationType":
			out.Values[i] = ec.marshalType(ctx, field.Selections, obj.MutationType())
		case "subscriptionType":
			out.Values[i] = ec.marshalType(ctx, field.Selections, obj.SubscriptionType())
		case "directives":
			out.Values[i] = ec.marshalDirectiveSlice(ctx, field.Selections, obj.Directives())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) handleType(ctx context.Context, sel ast.SelectionSet,
	obj *introspection.Type) graphql.Marshaler {
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Type"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Type")
		case "kind":
			out.Values[i] = ec.marshalN__TypeKind2string(ctx, field.Selections, obj.Kind())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "name":
			out.Values[i] = ec.marshalOString2ᚖstring(ctx, field.Selections, obj.Name())
		case "description":
			out.Values[i] = graphql.MarshalString(obj.Description())
		case "fields":
			out.Values[i] = ec.handleTypeFields(ctx, field, obj)
		case "interfaces":
			out.Values[i] = ec.marshalOptionalItypeSlice(ctx, field.Selections, obj.Interfaces())
		case "possibleTypes":
			out.Values[i] = ec.marshalOptionalItypeSlice(ctx, field.Selections, obj.PossibleTypes())
		case "enumValues":
			out.Values[i] = ec.handleTypeEnumValues(ctx, field, obj)
		case "inputFields":
			out.Values[i] = ec.marshalOptionalInputValueSlice(ctx, field.Selections,
				obj.InputFields())
		case "ofType":
			out.Values[i] = ec.marshalType(ctx, field.Selections, obj.OfType())
		default:
			panic("unknown field " + strconv.Quote(field.Name))
		}
	}
	out.Dispatch()
	if invalids > 0 {
		return graphql.Null
	}
	return out
}

func (ec *executionContext) marshalNBoolean2bool(ctx context.Context, sel ast.SelectionSet, v bool) graphql.Marshaler {
	res := graphql.MarshalBoolean(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalNString2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		// TODO - Check that resolver gets this information.
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalDirectiveSlice(ctx context.Context, sel ast.SelectionSet,
	v []introspection.Directive) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleDirective(ctx, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalN__DirectiveLocation2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalN__DirectiveLocation2ᚕstring(ctx context.Context, sel ast.SelectionSet, v []string) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalN__DirectiveLocation2string(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalN__EnumValue2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐEnumValue(ctx context.Context, sel ast.SelectionSet, v introspection.EnumValue) graphql.Marshaler {
	return ec.handleEnumValue(ctx, sel, &v)
}

func (ec *executionContext) marshalInputValueSlice(ctx context.Context, sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleInputValue(ctx, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalN__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v introspection.Type) graphql.Marshaler {
	return ec.handleType(ctx, sel, &v)
}

func (ec *executionContext) marshalIntrospectionTypeSlice(ctx context.Context, sel ast.SelectionSet, v []introspection.Type) graphql.Marshaler {
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.marshalN__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalIntrospectionType(ctx context.Context, sel ast.SelectionSet, v *introspection.Type) graphql.Marshaler {
	if v == nil {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
		return graphql.Null
	}
	return ec.handleType(ctx, sel, v)
}

func (ec *executionContext) marshalN__TypeKind2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalOString2ᚖstring(ctx context.Context, sel ast.SelectionSet, v *string) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return graphql.MarshalString(*v)
}

func (ec *executionContext) marshalOptionalEnumValueSlice(ctx context.Context,
	sel ast.SelectionSet, v []introspection.EnumValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleEnumValue(ctx, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalIntrospectionFieldSlice(ctx context.Context,
	sel ast.SelectionSet, v []introspection.Field) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.handleField(ctx, sel, &v[i])
	}
	return ret
}

func (ec *executionContext) marshalOptionalInputValueSlice(ctx context.Context,
	sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.handleInputValue(ctx, sel, &v[i])

	}
	return ret
}

func (ec *executionContext) marshalOptionalItypeSlice(ctx context.Context, sel ast.SelectionSet,
	v []introspection.Type) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.marshalN__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalType(ctx context.Context, sel ast.SelectionSet,
	v *introspection.Type) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return ec.handleType(ctx, sel, v)
}
