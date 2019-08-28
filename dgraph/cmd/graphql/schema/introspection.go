package schema

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/introspection"
	"github.com/vektah/gqlparser/ast"
)

func IntrospectionQuery(ctx context.Context, op *ast.OperationDefinition,
	schema *ast.Schema) *graphql.Response {
	ec := executionContext{graphql.GetRequestContext(ctx), schema}
	data := ec.handleQuery(ctx, op.SelectionSet)
	var buf bytes.Buffer
	data.MarshalGQL(&buf)
	d := buf.Bytes()

	return &graphql.Response{
		Data:       d,
		Errors:     ec.Errors,
		Extensions: ec.Extensions,
	}
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

func (ec *executionContext) ___Directive_args(ctx context.Context, field graphql.CollectedField, obj *introspection.Directive) (ret graphql.Marshaler) {
	res := obj.Args
	return ec.marshalN__InputValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx, field.Selections, res)
}

func (ec *executionContext) ___EnumValue_deprecationReason(ctx context.Context, field graphql.CollectedField, obj *introspection.EnumValue) (ret graphql.Marshaler) {
	res := obj.DeprecationReason()
	return ec.marshalOString2ᚖstring(ctx, field.Selections, res)
}

func (ec *executionContext) ___Field_args(ctx context.Context, field graphql.CollectedField, obj *introspection.Field) (ret graphql.Marshaler) {
	res := obj.Args
	return ec.marshalN__InputValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx, field.Selections, res)
}

func (ec *executionContext) ___Field_type(ctx context.Context, field graphql.CollectedField, obj *introspection.Field) (ret graphql.Marshaler) {
	res := obj.Type
	return ec.marshalN__Type2ᚖgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Field_deprecationReason(ctx context.Context, field graphql.CollectedField, obj *introspection.Field) (ret graphql.Marshaler) {
	res := obj.DeprecationReason()
	return ec.marshalOString2ᚖstring(ctx, field.Selections, res)
}

func (ec *executionContext) ___InputValue_type(ctx context.Context, field graphql.CollectedField, obj *introspection.InputValue) (ret graphql.Marshaler) {
	res := obj.Type
	return ec.marshalN__Type2ᚖgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___InputValue_defaultValue(ctx context.Context, field graphql.CollectedField, obj *introspection.InputValue) (ret graphql.Marshaler) {
	res := obj.DefaultValue
	return ec.marshalOString2ᚖstring(ctx, field.Selections, res)
}

func (ec *executionContext) ___Schema_types(ctx context.Context, field graphql.CollectedField, obj *introspection.Schema) (ret graphql.Marshaler) {
	res := obj.Types()
	return ec.marshalN__Type2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Schema_queryType(ctx context.Context, field graphql.CollectedField, obj *introspection.Schema) (ret graphql.Marshaler) {
	res := obj.QueryType()
	return ec.marshalN__Type2ᚖgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Schema_mutationType(ctx context.Context, field graphql.CollectedField, obj *introspection.Schema) (ret graphql.Marshaler) {
	res := obj.MutationType()
	return ec.marshalType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Schema_subscriptionType(ctx context.Context, field graphql.CollectedField, obj *introspection.Schema) (ret graphql.Marshaler) {
	res := obj.SubscriptionType()
	return ec.marshalType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Schema_directives(ctx context.Context, field graphql.CollectedField, obj *introspection.Schema) (ret graphql.Marshaler) {
	res := obj.Directives()
	return ec.marshalN__Directive2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐDirective(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_kind(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.Kind()
	return ec.marshalN__TypeKind2string(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_name(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.Name()
	return ec.marshalOString2ᚖstring(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_description(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.Description()
	return ec.marshalOString2string(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_fields(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.Fields(args["includeDeprecated"].(bool))
	return ec.marshalO__Field2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐField(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_interfaces(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.Interfaces()
	return ec.marshalO__Type2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_possibleTypes(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.PossibleTypes()
	return ec.marshalO__Type2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_enumValues(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	args := field.ArgumentMap(ec.Variables)
	res := obj.EnumValues(args["includeDeprecated"].(bool))
	if res == nil {
		return graphql.Null
	}
	return ec.marshalO__EnumValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐEnumValue(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_inputFields(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.InputFields()
	return ec.marshalO__InputValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx, field.Selections, res)
}

func (ec *executionContext) ___Type_ofType(ctx context.Context, field graphql.CollectedField, obj *introspection.Type) (ret graphql.Marshaler) {
	res := obj.OfType()
	return ec.marshalType(ctx, field.Selections, res)
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
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.Description)
		case "locations":
			out.Values[i] = ec.marshalN__DirectiveLocation2ᚕstring(ctx, field.Selections,
				obj.Locations)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "args":
			out.Values[i] = ec.___Directive_args(ctx, field, obj)
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
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.Description)
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections, obj.IsDeprecated())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "deprecationReason":
			out.Values[i] = ec.___EnumValue_deprecationReason(ctx, field, obj)
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
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.Description)
		case "args":
			out.Values[i] = ec.___Field_args(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "type":
			out.Values[i] = ec.___Field_type(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "isDeprecated":
			out.Values[i] = ec.marshalNBoolean2bool(ctx, field.Selections, obj.IsDeprecated())
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "deprecationReason":
			out.Values[i] = ec.___Field_deprecationReason(ctx, field, obj)
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
			out.Values[i] = ec.marshalOString2string(ctx, field.Selections, obj.Description)
		case "type":
			out.Values[i] = ec.___InputValue_type(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "defaultValue":
			out.Values[i] = ec.___InputValue_defaultValue(ctx, field, obj)
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
			out.Values[i] = ec.___Schema_types(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "queryType":
			out.Values[i] = ec.___Schema_queryType(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "mutationType":
			out.Values[i] = ec.___Schema_mutationType(ctx, field, obj)
		case "subscriptionType":
			out.Values[i] = ec.___Schema_subscriptionType(ctx, field, obj)
		case "directives":
			out.Values[i] = ec.___Schema_directives(ctx, field, obj)
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
	// TODO - See what does collect field really do?
	fields := graphql.CollectFields(ec.RequestContext, sel, []string{"__Type"})

	out := graphql.NewFieldSet(fields)
	var invalids uint32
	for i, field := range fields {
		switch field.Name {
		case "__typename":
			out.Values[i] = graphql.MarshalString("__Type")
		case "kind":
			out.Values[i] = ec.___Type_kind(ctx, field, obj)
			if out.Values[i] == graphql.Null {
				invalids++
			}
		case "name":
			out.Values[i] = ec.___Type_name(ctx, field, obj)
		case "description":
			out.Values[i] = ec.___Type_description(ctx, field, obj)
		case "fields":
			out.Values[i] = ec.___Type_fields(ctx, field, obj)
		case "interfaces":
			out.Values[i] = ec.___Type_interfaces(ctx, field, obj)
		case "possibleTypes":
			out.Values[i] = ec.___Type_possibleTypes(ctx, field, obj)
		case "enumValues":
			out.Values[i] = ec.___Type_enumValues(ctx, field, obj)
		case "inputFields":
			out.Values[i] = ec.___Type_inputFields(ctx, field, obj)
		case "ofType":
			out.Values[i] = ec.___Type_ofType(ctx, field, obj)
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

func (ec *executionContext) unmarshalNBoolean2bool(ctx context.Context, v interface{}) (bool, error) {
	return graphql.UnmarshalBoolean(v)
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

func (ec *executionContext) unmarshalNID2string(ctx context.Context, v interface{}) (string, error) {
	return graphql.UnmarshalID(v)
}

func (ec *executionContext) marshalNID2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalID(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) unmarshalNString2string(ctx context.Context, v interface{}) (string, error) {
	return graphql.UnmarshalString(v)
}

func (ec *executionContext) marshalNString2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	res := graphql.MarshalString(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) unmarshalNTime2timeᚐTime(ctx context.Context, v interface{}) (time.Time, error) {
	return graphql.UnmarshalTime(v)
}

func (ec *executionContext) marshalNTime2timeᚐTime(ctx context.Context, sel ast.SelectionSet, v time.Time) graphql.Marshaler {
	res := graphql.MarshalTime(v)
	if res == graphql.Null {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
	}
	return res
}

func (ec *executionContext) marshalN__Directive2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐDirective(ctx context.Context, sel ast.SelectionSet, v introspection.Directive) graphql.Marshaler {
	return ec.handleDirective(ctx, sel, &v)
}

func (ec *executionContext) marshalN__Directive2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐDirective(ctx context.Context, sel ast.SelectionSet, v []introspection.Directive) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalN__Directive2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐDirective(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) unmarshalN__DirectiveLocation2string(ctx context.Context, v interface{}) (string, error) {
	return graphql.UnmarshalString(v)
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

func (ec *executionContext) unmarshalN__DirectiveLocation2ᚕstring(ctx context.Context, v interface{}) ([]string, error) {
	var vSlice []interface{}
	if v != nil {
		if tmp1, ok := v.([]interface{}); ok {
			vSlice = tmp1
		} else {
			vSlice = []interface{}{v}
		}
	}
	var err error
	res := make([]string, len(vSlice))
	for i := range vSlice {
		res[i], err = ec.unmarshalN__DirectiveLocation2string(ctx, vSlice[i])
		if err != nil {
			return nil, err
		}
	}
	return res, nil
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

func (ec *executionContext) marshalN__Field2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐField(ctx context.Context, sel ast.SelectionSet, v introspection.Field) graphql.Marshaler {
	return ec.handleField(ctx, sel, &v)
}

func (ec *executionContext) marshalN__InputValue2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx context.Context, sel ast.SelectionSet, v introspection.InputValue) graphql.Marshaler {
	return ec.handleInputValue(ctx, sel, &v)
}

func (ec *executionContext) marshalN__InputValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx context.Context, sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalN__InputValue2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalN__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v introspection.Type) graphql.Marshaler {
	return ec.handleType(ctx, sel, &v)
}

func (ec *executionContext) marshalN__Type2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v []introspection.Type) graphql.Marshaler {
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.marshalN__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalN__Type2ᚖgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v *introspection.Type) graphql.Marshaler {
	if v == nil {
		if !ec.HasError(graphql.GetResolverContext(ctx)) {
			ec.Errorf(ctx, "must not be null")
		}
		return graphql.Null
	}
	return ec.handleType(ctx, sel, v)
}

func (ec *executionContext) unmarshalN__TypeKind2string(ctx context.Context, v interface{}) (string, error) {
	return graphql.UnmarshalString(v)
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

func (ec *executionContext) unmarshalOBoolean2bool(ctx context.Context, v interface{}) (bool, error) {
	return graphql.UnmarshalBoolean(v)
}

func (ec *executionContext) marshalOBoolean2bool(ctx context.Context, sel ast.SelectionSet, v bool) graphql.Marshaler {
	return graphql.MarshalBoolean(v)
}

func (ec *executionContext) unmarshalOBoolean2ᚖbool(ctx context.Context, v interface{}) (*bool, error) {
	if v == nil {
		return nil, nil
	}
	res, err := ec.unmarshalOBoolean2bool(ctx, v)
	return &res, err
}

func (ec *executionContext) marshalOBoolean2ᚖbool(ctx context.Context, sel ast.SelectionSet, v *bool) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return ec.marshalOBoolean2bool(ctx, sel, *v)
}

func (ec *executionContext) unmarshalOInt2int(ctx context.Context, v interface{}) (int, error) {
	return graphql.UnmarshalInt(v)
}

func (ec *executionContext) marshalOInt2int(ctx context.Context, sel ast.SelectionSet, v int) graphql.Marshaler {
	return graphql.MarshalInt(v)
}

func (ec *executionContext) unmarshalOInt2ᚖint(ctx context.Context, v interface{}) (*int, error) {
	if v == nil {
		return nil, nil
	}
	res, err := ec.unmarshalOInt2int(ctx, v)
	return &res, err
}

func (ec *executionContext) marshalOInt2ᚖint(ctx context.Context, sel ast.SelectionSet, v *int) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return ec.marshalOInt2int(ctx, sel, *v)
}

func (ec *executionContext) unmarshalOString2string(ctx context.Context, v interface{}) (string, error) {
	return graphql.UnmarshalString(v)
}

func (ec *executionContext) marshalOString2string(ctx context.Context, sel ast.SelectionSet, v string) graphql.Marshaler {
	return graphql.MarshalString(v)
}

func (ec *executionContext) unmarshalOString2ᚕstring(ctx context.Context, v interface{}) ([]string, error) {
	var vSlice []interface{}
	if v != nil {
		if tmp1, ok := v.([]interface{}); ok {
			vSlice = tmp1
		} else {
			vSlice = []interface{}{v}
		}
	}
	var err error
	res := make([]string, len(vSlice))
	for i := range vSlice {
		res[i], err = ec.unmarshalNString2string(ctx, vSlice[i])
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (ec *executionContext) marshalOString2ᚕstring(ctx context.Context, sel ast.SelectionSet, v []string) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalNString2string(ctx, sel, v[i])
	}

	return ret
}

func (ec *executionContext) unmarshalOString2ᚖstring(ctx context.Context, v interface{}) (*string, error) {
	if v == nil {
		return nil, nil
	}
	res, err := ec.unmarshalOString2string(ctx, v)
	return &res, err
}

func (ec *executionContext) marshalOString2ᚖstring(ctx context.Context, sel ast.SelectionSet, v *string) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return ec.marshalOString2string(ctx, sel, *v)
}

func (ec *executionContext) marshalO__EnumValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐEnumValue(ctx context.Context, sel ast.SelectionSet, v []introspection.EnumValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalN__EnumValue2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐEnumValue(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalO__Field2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐField(ctx context.Context, sel ast.SelectionSet, v []introspection.Field) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))

	for i := range v {
		ret[i] = ec.marshalN__Field2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐField(ctx, sel, v[i])
	}
	return ret
}

func (ec *executionContext) marshalO__InputValue2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx context.Context, sel ast.SelectionSet, v []introspection.InputValue) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	ret := make(graphql.Array, len(v))
	for i := range v {
		ret[i] = ec.marshalN__InputValue2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐInputValue(ctx, sel, v[i])

	}
	return ret
}

func (ec *executionContext) marshalO__Schema2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐSchema(ctx context.Context, sel ast.SelectionSet, v introspection.Schema) graphql.Marshaler {
	return ec.handleSchema(ctx, sel, &v)
}

func (ec *executionContext) marshalO__Schema2ᚖgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐSchema(ctx context.Context, sel ast.SelectionSet, v *introspection.Schema) graphql.Marshaler {
	if v == nil {
		return graphql.Null
	}
	return ec.handleSchema(ctx, sel, v)
}

func (ec *executionContext) marshalO__Type2githubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v introspection.Type) graphql.Marshaler {
	return ec.handleType(ctx, sel, &v)
}

func (ec *executionContext) marshalO__Type2ᚕgithubᚗcomᚋ99designsᚋgqlgenᚋgraphqlᚋintrospectionᚐType(ctx context.Context, sel ast.SelectionSet, v []introspection.Type) graphql.Marshaler {
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
