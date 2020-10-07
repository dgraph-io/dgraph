/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"bufio"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// A Handler can produce valid GraphQL and Dgraph schemas given an input of
// types and relationships
type Handler interface {
	DGSchema() string
	GQLSchema() string
}

type handler struct {
	input          string
	originalDefs   []string
	completeSchema *ast.Schema
	dgraphSchema   string
}

// FromString builds a GraphQL Schema from input string, or returns any parsing
// or validation errors.
func FromString(schema string) (Schema, error) {
	// validator.Prelude includes a bunch of predefined types which help with schema introspection
	// queries, hence we include it as part of the schema.
	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: schema})
	if gqlErr != nil {
		return nil, errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	return AsSchema(gqlSchema)
}

func (s *handler) GQLSchema() string {
	return Stringify(s.completeSchema, s.originalDefs)
}

func (s *handler) DGSchema() string {
	return s.dgraphSchema
}

func parseSecrets(sch string) (map[string]string, *authorization.AuthMeta, error) {
	m := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(sch))
	authSecret := ""
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(text, "# Dgraph.Authorization") {
			if authSecret != "" {
				return nil, nil, errors.Errorf("Dgraph.Authorization should be only be specified once in "+
					"a schema, found second mention: %v", text)
			}
			authSecret = text
			continue
		}
		if !strings.HasPrefix(text, "# Dgraph.Secret") {
			continue
		}
		parts := strings.Fields(text)
		const doubleQuotesCode = 34

		if len(parts) < 4 {
			return nil, nil, errors.Errorf("incorrect format for specifying Dgraph secret found for "+
				"comment: `%s`, it should be `# Dgraph.Secret key value`", text)
		}
		val := strings.Join(parts[3:], " ")
		if strings.Count(val, `"`) != 2 || val[0] != doubleQuotesCode || val[len(val)-1] != doubleQuotesCode {
			return nil, nil, errors.Errorf("incorrect format for specifying Dgraph secret found for "+
				"comment: `%s`, it should be `# Dgraph.Secret key value`", text)
		}

		val = strings.Trim(val, `"`)
		key := strings.Trim(parts[2], `"`)
		m[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, errors.Wrapf(err, "while trying to parse secrets from schema file")
	}

	if authSecret == "" {
		return m, nil, nil
	}

	metaInfo, err := authorization.ParseAuthMeta(authSecret)
	if err != nil {
		return nil, nil, err
	}
	return m, metaInfo, nil
}

// NewHandler processes the input schema. If there are no errors, it returns
// a valid Handler, otherwise it returns nil and an error.
func NewHandler(input string, validateOnly bool) (Handler, error) {
	if input == "" {
		return nil, gqlerror.Errorf("No schema specified")
	}

	secrets, metaInfo, err := parseSecrets(input)
	if err != nil {
		return nil, err
	}
	// lets obfuscate the value of the secrets from here on.
	schemaSecrets := make(map[string]x.SensitiveByteSlice, len(secrets))
	for k, v := range secrets {
		schemaSecrets[k] = x.SensitiveByteSlice([]byte(v))
	}

	// The input schema contains just what's required to describe the types,
	// relationships and searchability - but that's not enough to define a
	// valid GraphQL schema: e.g. we allow an input schema file like
	//
	// type T {
	//   f: Int @search
	// }
	//
	// But, that's not valid GraphQL unless there's also definitions of scalars
	// (Int, String, etc) and definitions of the directives (@search, etc).
	// We don't want to make the user have those in their file and then we have
	// to check that they've made the right definitions, etc, etc.
	//
	// So we parse the original input of just types and relationships and
	// run a validation to make sure it only contains things that it should.
	// To that we add all the scalars and other definitions we always require.
	//
	// Then, we GraphQL validate to make sure their definitions plus our additions
	// is GraphQL valid.  At this point we know the definitions are GraphQL valid,
	// but we need to check if it makes sense to our layer.
	//
	// The next final validation ensures that the definitions are made
	// in such a way that our GraphQL API will be able to interpret the schema
	// correctly.
	//
	// Then we can complete the process by adding in queries and mutations etc. to
	// make the final full GraphQL schema.

	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: input})
	if gqlErr != nil {
		return nil, gqlerror.List{gqlErr}
	}

	gqlErrList := preGQLValidation(doc)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	typesToComplete := make([]string, 0, len(doc.Definitions))
	defns := make([]string, 0, len(doc.Definitions))
	for _, defn := range doc.Definitions {
		if defn.BuiltIn {
			continue
		}
		defns = append(defns, defn.Name)
		if defn.Kind == ast.Object || defn.Kind == ast.Interface {
			remoteDir := defn.Directives.ForName(remoteDirective)
			if remoteDir != nil {
				continue
			}
		}
		typesToComplete = append(typesToComplete, defn.Name)
	}

	expandSchema(doc)

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, gqlerror.List{gqlErr}
	}

	gqlErrList = postGQLValidation(sch, defns, schemaSecrets)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	var authHeader string
	if metaInfo != nil {
		authHeader = metaInfo.Header
	}

	headers := getAllowedHeaders(sch, defns, authHeader)
	dgSchema := genDgSchema(sch, typesToComplete)
	completeSchema(sch, typesToComplete)
	cleanSchema(sch)

	if len(sch.Query.Fields) == 0 && len(sch.Mutation.Fields) == 0 {
		return nil, gqlerror.Errorf("No query or mutation found in the generated schema")
	}

	// If Dgraph.Authorization header is parsed successfully and JWKUrl is present
	// then Fetch the JWKs from the JWKUrl
	if metaInfo != nil && metaInfo.JWKUrl != "" {
		fetchErr := metaInfo.FetchJWKs()
		if fetchErr != nil {
			return nil, fetchErr
		}
	}

	handler := &handler{
		input:          input,
		dgraphSchema:   dgSchema,
		completeSchema: sch,
		originalDefs:   defns,
	}

	// Return early since we are only validating the schema.
	if validateOnly {
		return handler, nil
	}

	hc.Lock()
	hc.allowed = headers
	hc.secrets = schemaSecrets
	hc.Unlock()

	if metaInfo != nil {
		authorization.SetAuthMeta(metaInfo)
	}
	return handler, nil
}

type headersConfig struct {
	// comma separated list of allowed headers. These are parsed from the forwardHeaders specified
	// in the @custom directive. They are returned to the client as part of
	// Access-Control-Allow-Headers.
	allowed string
	// secrets are key value pairs stored in the GraphQL schema which can be added as headers
	// to requests which resolve custom queries/mutations.
	secrets map[string]x.SensitiveByteSlice
	sync.RWMutex
}

var hc = headersConfig{
	allowed: x.AccessControlAllowedHeaders,
}

func getAllowedHeaders(sch *ast.Schema, definitions []string, authHeader string) string {
	headers := make(map[string]struct{})

	setHeaders := func(dir *ast.Directive) {
		if dir == nil {
			return
		}

		httpArg := dir.Arguments.ForName("http")
		if httpArg == nil {
			return
		}
		forwardHeaders := httpArg.Value.Children.ForName("forwardHeaders")
		if forwardHeaders == nil {
			return
		}
		for _, h := range forwardHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) == 1 {
				key = []string{h.Value.Raw, h.Value.Raw}
			}
			headers[key[1]] = struct{}{}
		}
	}

	for _, defn := range definitions {
		typ := sch.Types[defn]
		custom := typ.Directives.ForName(customDirective)
		setHeaders(custom)
		for _, field := range typ.Fields {
			custom := field.Directives.ForName(customDirective)
			setHeaders(custom)
		}
	}

	finalHeaders := make([]string, 0, len(headers)+1)
	for h := range headers {
		finalHeaders = append(finalHeaders, h)
	}

	// Add Auth Header to allowed headers list
	if authHeader != "" {
		finalHeaders = append(finalHeaders, authHeader)
	}

	allowed := x.AccessControlAllowedHeaders
	customHeaders := strings.Join(finalHeaders, ",")
	if len(customHeaders) > 0 {
		allowed += "," + customHeaders
	}

	return allowed
}

func AllowedHeaders() string {
	hc.RLock()
	defer hc.RUnlock()
	return hc.allowed
}

func getAllSearchIndexes(val *ast.Value) []string {
	res := make([]string, len(val.Children))

	for i, child := range val.Children {
		res[i] = supportedSearches[child.Value.Raw].dgIndex
	}

	return res
}

func typeName(def *ast.Definition) string {
	name := def.Name
	dir := def.Directives.ForName(dgraphDirective)
	if dir == nil {
		return name
	}
	typeArg := dir.Arguments.ForName(dgraphTypeArg)
	if typeArg == nil {
		return name
	}
	return typeArg.Value.Raw
}

// fieldName returns the dgraph predicate corresponding to a field.
// If the field had a dgraph directive, then it returns the value of the pred arg otherwise
// it returns typeName + "." + fieldName.
func fieldName(def *ast.FieldDefinition, typName string) string {
	predArg := getDgraphDirPredArg(def)
	if predArg == nil {
		return typName + "." + def.Name
	}
	return predArg.Value.Raw
}

func getDgraphDirPredArg(def *ast.FieldDefinition) *ast.Argument {
	dir := def.Directives.ForName(dgraphDirective)
	if dir == nil {
		return nil
	}
	predArg := dir.Arguments.ForName(dgraphPredArg)
	return predArg
}

// genDgSchema generates Dgraph schema from a valid graphql schema.
func genDgSchema(gqlSch *ast.Schema, definitions []string) string {
	var typeStrings []string

	type dgPred struct {
		typ     string
		indexes map[string]bool
		upsert  string
		reverse string
	}

	type field struct {
		name string
		// true if the field was inherited from an interface, we don't add the predicate schema
		// for it then as the it would already have been added with the interface.
		inherited bool
	}

	type dgType struct {
		name   string
		fields []field
	}

	dgTypes := make([]dgType, 0, len(definitions))
	dgPreds := make(map[string]dgPred)

	getUpdatedPred := func(fname, typStr, upsertStr string, indexes []string) dgPred {
		pred, ok := dgPreds[fname]
		if !ok {
			pred = dgPred{
				typ:     typStr,
				indexes: make(map[string]bool),
				upsert:  upsertStr,
			}
		}
		for _, index := range indexes {
			pred.indexes[index] = true
		}
		return pred
	}

	for _, key := range definitions {
		if isQueryOrMutation(key) {
			continue
		}
		def := gqlSch.Types[key]
		switch def.Kind {
		case ast.Object, ast.Interface:
			typName := typeName(def)

			typ := dgType{name: typName}
			pwdField := getPasswordField(def)

			for _, f := range def.Fields {
				if f.Type.Name() == "ID" || hasCustomOrLambda(f) {
					continue
				}

				typName = typeName(def)
				// This field could have originally been defined in an interface that this type
				// implements. If we get a parent interface, then we should prefix the field name
				// with it instead of def.Name.
				parentInt := parentInterface(gqlSch, def, f.Name)
				if parentInt != nil {
					typName = typeName(parentInt)
				}
				fname := fieldName(f, typName)

				var prefix, suffix string
				if f.Type.Elem != nil {
					prefix = "["
					suffix = "]"
				}

				var typStr string
				switch gqlSch.Types[f.Type.Name()].Kind {
				case ast.Object, ast.Interface:
					if isPointType(f.Type) {
						typStr = "geo"
						var indexes []string
						if f.Directives.ForName(searchDirective) != nil {
							indexes = append(indexes, "geo")
						}
						dgPreds[fname] = getUpdatedPred(fname, typStr, "", indexes)
					} else {
						typStr = fmt.Sprintf("%suid%s", prefix, suffix)
					}

					if parentInt == nil {
						if strings.HasPrefix(fname, "~") {
							// remove ~
							forwardEdge := fname[1:]
							forwardPred := dgPreds[forwardEdge]
							forwardPred.reverse = "@reverse "
							dgPreds[forwardEdge] = forwardPred
						} else {
							pred := dgPreds[fname]
							pred.typ = typStr
							dgPreds[fname] = pred
						}
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				case ast.Scalar:
					typStr = fmt.Sprintf(
						"%s%s%s",
						prefix, scalarToDgraph[f.Type.Name()], suffix,
					)

					var indexes []string
					upsertStr := ""
					search := f.Directives.ForName(searchDirective)
					id := f.Directives.ForName(idDirective)
					if id != nil {
						upsertStr = "@upsert "
						indexes = append(indexes, "hash")
					}

					if search != nil {
						arg := search.Arguments.ForName(searchArgs)
						if arg != nil {
							indexes = append(indexes, getAllSearchIndexes(arg.Value)...)
						} else {
							indexes = append(indexes, supportedSearches[defaultSearches[f.Type.
								Name()]].dgIndex)
						}
					}

					if parentInt == nil {
						dgPreds[fname] = getUpdatedPred(fname, typStr, upsertStr, indexes)
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				case ast.Enum:
					typStr = fmt.Sprintf("%s%s%s", prefix, "string", suffix)

					indexes := []string{"hash"}
					search := f.Directives.ForName(searchDirective)
					if search != nil {
						arg := search.Arguments.ForName(searchArgs)
						if arg != nil {
							indexes = getAllSearchIndexes(arg.Value)
						}
					}
					if parentInt == nil {
						dgPreds[fname] = getUpdatedPred(fname, typStr, "", indexes)
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				}
			}
			if pwdField != nil {
				parentInt := parentInterfaceForPwdField(gqlSch, def, pwdField.Name)
				if parentInt != nil {
					typName = typeName(parentInt)
				}
				fname := fieldName(pwdField, typName)

				if parentInt == nil {
					dgPreds[fname] = dgPred{typ: "password"}
				}

				typ.fields = append(typ.fields, field{fname, parentInt != nil})
			}
			dgTypes = append(dgTypes, typ)
		}
	}

	predWritten := make(map[string]bool, len(dgPreds))
	for _, typ := range dgTypes {
		var typeDef, preds strings.Builder
		fmt.Fprintf(&typeDef, "type %s {\n", typ.name)
		for _, fld := range typ.fields {
			f, ok := dgPreds[fld.name]
			if !ok {
				continue
			}
			fmt.Fprintf(&typeDef, "  %s\n", fld.name)
			if !fld.inherited && !predWritten[fld.name] {
				indexStr := ""
				if len(f.indexes) > 0 {
					indexes := make([]string, 0)
					for index := range f.indexes {
						indexes = append(indexes, index)
					}
					sort.Strings(indexes)
					indexStr = fmt.Sprintf(" @index(%s)", strings.Join(indexes, ", "))
				}
				fmt.Fprintf(&preds, "%s: %s%s %s%s.\n", fld.name, f.typ, indexStr, f.upsert,
					f.reverse)
				predWritten[fld.name] = true
			}
		}
		fmt.Fprintf(&typeDef, "}\n")
		typeStrings = append(
			typeStrings,
			fmt.Sprintf("%s%s", typeDef.String(), preds.String()),
		)
	}

	return strings.Join(typeStrings, "")
}
