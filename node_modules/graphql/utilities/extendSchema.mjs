function ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); keys.push.apply(keys, symbols); } return keys; }

function _objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { ownKeys(Object(source), true).forEach(function (key) { _defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function _defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

import objectValues from "../polyfills/objectValues.mjs";
import keyMap from "../jsutils/keyMap.mjs";
import inspect from "../jsutils/inspect.mjs";
import mapValue from "../jsutils/mapValue.mjs";
import invariant from "../jsutils/invariant.mjs";
import devAssert from "../jsutils/devAssert.mjs";
import { Kind } from "../language/kinds.mjs";
import { TokenKind } from "../language/tokenKind.mjs";
import { dedentBlockStringValue } from "../language/blockString.mjs";
import { isTypeDefinitionNode, isTypeExtensionNode } from "../language/predicates.mjs";
import { assertValidSDLExtension } from "../validation/validate.mjs";
import { getDirectiveValues } from "../execution/values.mjs";
import { assertSchema, GraphQLSchema } from "../type/schema.mjs";
import { specifiedScalarTypes, isSpecifiedScalarType } from "../type/scalars.mjs";
import { introspectionTypes, isIntrospectionType } from "../type/introspection.mjs";
import { GraphQLDirective, GraphQLDeprecatedDirective, GraphQLSpecifiedByDirective } from "../type/directives.mjs";
import { isScalarType, isObjectType, isInterfaceType, isUnionType, isListType, isNonNullType, isEnumType, isInputObjectType, GraphQLList, GraphQLNonNull, GraphQLScalarType, GraphQLObjectType, GraphQLInterfaceType, GraphQLUnionType, GraphQLEnumType, GraphQLInputObjectType } from "../type/definition.mjs";
import { valueFromAST } from "./valueFromAST.mjs";

/**
 * Produces a new schema given an existing schema and a document which may
 * contain GraphQL type extensions and definitions. The original schema will
 * remain unaltered.
 *
 * Because a schema represents a graph of references, a schema cannot be
 * extended without effectively making an entire copy. We do not know until it's
 * too late if subgraphs remain unchanged.
 *
 * This algorithm copies the provided schema, applying extensions while
 * producing the copy. The original schema remains unaltered.
 *
 * Accepts options as a third argument:
 *
 *    - commentDescriptions:
 *        Provide true to use preceding comments as the description.
 *
 */
export function extendSchema(schema, documentAST, options) {
  assertSchema(schema);
  documentAST != null && documentAST.kind === Kind.DOCUMENT || devAssert(0, 'Must provide valid Document AST.');

  if ((options === null || options === void 0 ? void 0 : options.assumeValid) !== true && (options === null || options === void 0 ? void 0 : options.assumeValidSDL) !== true) {
    assertValidSDLExtension(documentAST, schema);
  }

  var schemaConfig = schema.toConfig();
  var extendedConfig = extendSchemaImpl(schemaConfig, documentAST, options);
  return schemaConfig === extendedConfig ? schema : new GraphQLSchema(extendedConfig);
}
/**
 * @internal
 */

export function extendSchemaImpl(schemaConfig, documentAST, options) {
  var _schemaDef, _schemaDef$descriptio, _schemaDef2, _options$assumeValid;

  // Collect the type definitions and extensions found in the document.
  var typeDefs = [];
  var typeExtensionsMap = Object.create(null); // New directives and types are separate because a directives and types can
  // have the same name. For example, a type named "skip".

  var directiveDefs = [];
  var schemaDef; // Schema extensions are collected which may add additional operation types.

  var schemaExtensions = [];

  for (var _i2 = 0, _documentAST$definiti2 = documentAST.definitions; _i2 < _documentAST$definiti2.length; _i2++) {
    var def = _documentAST$definiti2[_i2];

    if (def.kind === Kind.SCHEMA_DEFINITION) {
      schemaDef = def;
    } else if (def.kind === Kind.SCHEMA_EXTENSION) {
      schemaExtensions.push(def);
    } else if (isTypeDefinitionNode(def)) {
      typeDefs.push(def);
    } else if (isTypeExtensionNode(def)) {
      var extendedTypeName = def.name.value;
      var existingTypeExtensions = typeExtensionsMap[extendedTypeName];
      typeExtensionsMap[extendedTypeName] = existingTypeExtensions ? existingTypeExtensions.concat([def]) : [def];
    } else if (def.kind === Kind.DIRECTIVE_DEFINITION) {
      directiveDefs.push(def);
    }
  } // If this document contains no new types, extensions, or directives then
  // return the same unmodified GraphQLSchema instance.


  if (Object.keys(typeExtensionsMap).length === 0 && typeDefs.length === 0 && directiveDefs.length === 0 && schemaExtensions.length === 0 && schemaDef == null) {
    return schemaConfig;
  }

  var typeMap = Object.create(null);

  for (var _i4 = 0, _schemaConfig$types2 = schemaConfig.types; _i4 < _schemaConfig$types2.length; _i4++) {
    var existingType = _schemaConfig$types2[_i4];
    typeMap[existingType.name] = extendNamedType(existingType);
  }

  for (var _i6 = 0; _i6 < typeDefs.length; _i6++) {
    var _stdTypeMap$name;

    var typeNode = typeDefs[_i6];
    var name = typeNode.name.value;
    typeMap[name] = (_stdTypeMap$name = stdTypeMap[name]) !== null && _stdTypeMap$name !== void 0 ? _stdTypeMap$name : buildType(typeNode);
  }

  var operationTypes = _objectSpread(_objectSpread({
    // Get the extended root operation types.
    query: schemaConfig.query && replaceNamedType(schemaConfig.query),
    mutation: schemaConfig.mutation && replaceNamedType(schemaConfig.mutation),
    subscription: schemaConfig.subscription && replaceNamedType(schemaConfig.subscription)
  }, schemaDef && getOperationTypes([schemaDef])), getOperationTypes(schemaExtensions)); // Then produce and return a Schema config with these types.


  return _objectSpread(_objectSpread({
    description: (_schemaDef = schemaDef) === null || _schemaDef === void 0 ? void 0 : (_schemaDef$descriptio = _schemaDef.description) === null || _schemaDef$descriptio === void 0 ? void 0 : _schemaDef$descriptio.value
  }, operationTypes), {}, {
    types: objectValues(typeMap),
    directives: [].concat(schemaConfig.directives.map(replaceDirective), directiveDefs.map(buildDirective)),
    extensions: undefined,
    astNode: (_schemaDef2 = schemaDef) !== null && _schemaDef2 !== void 0 ? _schemaDef2 : schemaConfig.astNode,
    extensionASTNodes: schemaConfig.extensionASTNodes.concat(schemaExtensions),
    assumeValid: (_options$assumeValid = options === null || options === void 0 ? void 0 : options.assumeValid) !== null && _options$assumeValid !== void 0 ? _options$assumeValid : false
  }); // Below are functions used for producing this schema that have closed over
  // this scope and have access to the schema, cache, and newly defined types.

  function replaceType(type) {
    if (isListType(type)) {
      return new GraphQLList(replaceType(type.ofType));
    } else if (isNonNullType(type)) {
      return new GraphQLNonNull(replaceType(type.ofType));
    }

    return replaceNamedType(type);
  }

  function replaceNamedType(type) {
    // Note: While this could make early assertions to get the correctly
    // typed values, that would throw immediately while type system
    // validation with validateSchema() will produce more actionable results.
    return typeMap[type.name];
  }

  function replaceDirective(directive) {
    var config = directive.toConfig();
    return new GraphQLDirective(_objectSpread(_objectSpread({}, config), {}, {
      args: mapValue(config.args, extendArg)
    }));
  }

  function extendNamedType(type) {
    if (isIntrospectionType(type) || isSpecifiedScalarType(type)) {
      // Builtin types are not extended.
      return type;
    }

    if (isScalarType(type)) {
      return extendScalarType(type);
    }

    if (isObjectType(type)) {
      return extendObjectType(type);
    }

    if (isInterfaceType(type)) {
      return extendInterfaceType(type);
    }

    if (isUnionType(type)) {
      return extendUnionType(type);
    }

    if (isEnumType(type)) {
      return extendEnumType(type);
    } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


    if (isInputObjectType(type)) {
      return extendInputObjectType(type);
    } // istanbul ignore next (Not reachable. All possible types have been considered)


    false || invariant(0, 'Unexpected type: ' + inspect(type));
  }

  function extendInputObjectType(type) {
    var _typeExtensionsMap$co;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$co = typeExtensionsMap[config.name]) !== null && _typeExtensionsMap$co !== void 0 ? _typeExtensionsMap$co : [];
    return new GraphQLInputObjectType(_objectSpread(_objectSpread({}, config), {}, {
      fields: function fields() {
        return _objectSpread(_objectSpread({}, mapValue(config.fields, function (field) {
          return _objectSpread(_objectSpread({}, field), {}, {
            type: replaceType(field.type)
          });
        })), buildInputFieldMap(extensions));
      },
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendEnumType(type) {
    var _typeExtensionsMap$ty;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$ty = typeExtensionsMap[type.name]) !== null && _typeExtensionsMap$ty !== void 0 ? _typeExtensionsMap$ty : [];
    return new GraphQLEnumType(_objectSpread(_objectSpread({}, config), {}, {
      values: _objectSpread(_objectSpread({}, config.values), buildEnumValueMap(extensions)),
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendScalarType(type) {
    var _typeExtensionsMap$co2;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$co2 = typeExtensionsMap[config.name]) !== null && _typeExtensionsMap$co2 !== void 0 ? _typeExtensionsMap$co2 : [];
    var specifiedByUrl = config.specifiedByUrl;

    for (var _i8 = 0; _i8 < extensions.length; _i8++) {
      var _getSpecifiedByUrl;

      var extensionNode = extensions[_i8];
      specifiedByUrl = (_getSpecifiedByUrl = getSpecifiedByUrl(extensionNode)) !== null && _getSpecifiedByUrl !== void 0 ? _getSpecifiedByUrl : specifiedByUrl;
    }

    return new GraphQLScalarType(_objectSpread(_objectSpread({}, config), {}, {
      specifiedByUrl: specifiedByUrl,
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendObjectType(type) {
    var _typeExtensionsMap$co3;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$co3 = typeExtensionsMap[config.name]) !== null && _typeExtensionsMap$co3 !== void 0 ? _typeExtensionsMap$co3 : [];
    return new GraphQLObjectType(_objectSpread(_objectSpread({}, config), {}, {
      interfaces: function interfaces() {
        return [].concat(type.getInterfaces().map(replaceNamedType), buildInterfaces(extensions));
      },
      fields: function fields() {
        return _objectSpread(_objectSpread({}, mapValue(config.fields, extendField)), buildFieldMap(extensions));
      },
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendInterfaceType(type) {
    var _typeExtensionsMap$co4;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$co4 = typeExtensionsMap[config.name]) !== null && _typeExtensionsMap$co4 !== void 0 ? _typeExtensionsMap$co4 : [];
    return new GraphQLInterfaceType(_objectSpread(_objectSpread({}, config), {}, {
      interfaces: function interfaces() {
        return [].concat(type.getInterfaces().map(replaceNamedType), buildInterfaces(extensions));
      },
      fields: function fields() {
        return _objectSpread(_objectSpread({}, mapValue(config.fields, extendField)), buildFieldMap(extensions));
      },
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendUnionType(type) {
    var _typeExtensionsMap$co5;

    var config = type.toConfig();
    var extensions = (_typeExtensionsMap$co5 = typeExtensionsMap[config.name]) !== null && _typeExtensionsMap$co5 !== void 0 ? _typeExtensionsMap$co5 : [];
    return new GraphQLUnionType(_objectSpread(_objectSpread({}, config), {}, {
      types: function types() {
        return [].concat(type.getTypes().map(replaceNamedType), buildUnionTypes(extensions));
      },
      extensionASTNodes: config.extensionASTNodes.concat(extensions)
    }));
  }

  function extendField(field) {
    return _objectSpread(_objectSpread({}, field), {}, {
      type: replaceType(field.type),
      args: mapValue(field.args, extendArg)
    });
  }

  function extendArg(arg) {
    return _objectSpread(_objectSpread({}, arg), {}, {
      type: replaceType(arg.type)
    });
  }

  function getOperationTypes(nodes) {
    var opTypes = {};

    for (var _i10 = 0; _i10 < nodes.length; _i10++) {
      var _node$operationTypes;

      var node = nodes[_i10];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var operationTypesNodes = (_node$operationTypes = node.operationTypes) !== null && _node$operationTypes !== void 0 ? _node$operationTypes : [];

      for (var _i12 = 0; _i12 < operationTypesNodes.length; _i12++) {
        var operationType = operationTypesNodes[_i12];
        opTypes[operationType.operation] = getNamedType(operationType.type);
      }
    } // Note: While this could make early assertions to get the correctly
    // typed values below, that would throw immediately while type system
    // validation with validateSchema() will produce more actionable results.


    return opTypes;
  }

  function getNamedType(node) {
    var _stdTypeMap$name2;

    var name = node.name.value;
    var type = (_stdTypeMap$name2 = stdTypeMap[name]) !== null && _stdTypeMap$name2 !== void 0 ? _stdTypeMap$name2 : typeMap[name];

    if (type === undefined) {
      throw new Error("Unknown type: \"".concat(name, "\"."));
    }

    return type;
  }

  function getWrappedType(node) {
    if (node.kind === Kind.LIST_TYPE) {
      return new GraphQLList(getWrappedType(node.type));
    }

    if (node.kind === Kind.NON_NULL_TYPE) {
      return new GraphQLNonNull(getWrappedType(node.type));
    }

    return getNamedType(node);
  }

  function buildDirective(node) {
    var locations = node.locations.map(function (_ref) {
      var value = _ref.value;
      return value;
    });
    return new GraphQLDirective({
      name: node.name.value,
      description: getDescription(node, options),
      locations: locations,
      isRepeatable: node.repeatable,
      args: buildArgumentMap(node.arguments),
      astNode: node
    });
  }

  function buildFieldMap(nodes) {
    var fieldConfigMap = Object.create(null);

    for (var _i14 = 0; _i14 < nodes.length; _i14++) {
      var _node$fields;

      var node = nodes[_i14];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var nodeFields = (_node$fields = node.fields) !== null && _node$fields !== void 0 ? _node$fields : [];

      for (var _i16 = 0; _i16 < nodeFields.length; _i16++) {
        var field = nodeFields[_i16];
        fieldConfigMap[field.name.value] = {
          // Note: While this could make assertions to get the correctly typed
          // value, that would throw immediately while type system validation
          // with validateSchema() will produce more actionable results.
          type: getWrappedType(field.type),
          description: getDescription(field, options),
          args: buildArgumentMap(field.arguments),
          deprecationReason: getDeprecationReason(field),
          astNode: field
        };
      }
    }

    return fieldConfigMap;
  }

  function buildArgumentMap(args) {
    // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
    var argsNodes = args !== null && args !== void 0 ? args : [];
    var argConfigMap = Object.create(null);

    for (var _i18 = 0; _i18 < argsNodes.length; _i18++) {
      var arg = argsNodes[_i18];
      // Note: While this could make assertions to get the correctly typed
      // value, that would throw immediately while type system validation
      // with validateSchema() will produce more actionable results.
      var type = getWrappedType(arg.type);
      argConfigMap[arg.name.value] = {
        type: type,
        description: getDescription(arg, options),
        defaultValue: valueFromAST(arg.defaultValue, type),
        astNode: arg
      };
    }

    return argConfigMap;
  }

  function buildInputFieldMap(nodes) {
    var inputFieldMap = Object.create(null);

    for (var _i20 = 0; _i20 < nodes.length; _i20++) {
      var _node$fields2;

      var node = nodes[_i20];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var fieldsNodes = (_node$fields2 = node.fields) !== null && _node$fields2 !== void 0 ? _node$fields2 : [];

      for (var _i22 = 0; _i22 < fieldsNodes.length; _i22++) {
        var field = fieldsNodes[_i22];
        // Note: While this could make assertions to get the correctly typed
        // value, that would throw immediately while type system validation
        // with validateSchema() will produce more actionable results.
        var type = getWrappedType(field.type);
        inputFieldMap[field.name.value] = {
          type: type,
          description: getDescription(field, options),
          defaultValue: valueFromAST(field.defaultValue, type),
          astNode: field
        };
      }
    }

    return inputFieldMap;
  }

  function buildEnumValueMap(nodes) {
    var enumValueMap = Object.create(null);

    for (var _i24 = 0; _i24 < nodes.length; _i24++) {
      var _node$values;

      var node = nodes[_i24];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var valuesNodes = (_node$values = node.values) !== null && _node$values !== void 0 ? _node$values : [];

      for (var _i26 = 0; _i26 < valuesNodes.length; _i26++) {
        var value = valuesNodes[_i26];
        enumValueMap[value.name.value] = {
          description: getDescription(value, options),
          deprecationReason: getDeprecationReason(value),
          astNode: value
        };
      }
    }

    return enumValueMap;
  }

  function buildInterfaces(nodes) {
    var interfaces = [];

    for (var _i28 = 0; _i28 < nodes.length; _i28++) {
      var _node$interfaces;

      var node = nodes[_i28];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var interfacesNodes = (_node$interfaces = node.interfaces) !== null && _node$interfaces !== void 0 ? _node$interfaces : [];

      for (var _i30 = 0; _i30 < interfacesNodes.length; _i30++) {
        var type = interfacesNodes[_i30];
        // Note: While this could make assertions to get the correctly typed
        // values below, that would throw immediately while type system
        // validation with validateSchema() will produce more actionable
        // results.
        interfaces.push(getNamedType(type));
      }
    }

    return interfaces;
  }

  function buildUnionTypes(nodes) {
    var types = [];

    for (var _i32 = 0; _i32 < nodes.length; _i32++) {
      var _node$types;

      var node = nodes[_i32];
      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var typeNodes = (_node$types = node.types) !== null && _node$types !== void 0 ? _node$types : [];

      for (var _i34 = 0; _i34 < typeNodes.length; _i34++) {
        var type = typeNodes[_i34];
        // Note: While this could make assertions to get the correctly typed
        // values below, that would throw immediately while type system
        // validation with validateSchema() will produce more actionable
        // results.
        types.push(getNamedType(type));
      }
    }

    return types;
  }

  function buildType(astNode) {
    var _typeExtensionsMap$na;

    var name = astNode.name.value;
    var description = getDescription(astNode, options);
    var extensionNodes = (_typeExtensionsMap$na = typeExtensionsMap[name]) !== null && _typeExtensionsMap$na !== void 0 ? _typeExtensionsMap$na : [];

    switch (astNode.kind) {
      case Kind.OBJECT_TYPE_DEFINITION:
        {
          var extensionASTNodes = extensionNodes;
          var allNodes = [astNode].concat(extensionASTNodes);
          return new GraphQLObjectType({
            name: name,
            description: description,
            interfaces: function interfaces() {
              return buildInterfaces(allNodes);
            },
            fields: function fields() {
              return buildFieldMap(allNodes);
            },
            astNode: astNode,
            extensionASTNodes: extensionASTNodes
          });
        }

      case Kind.INTERFACE_TYPE_DEFINITION:
        {
          var _extensionASTNodes = extensionNodes;

          var _allNodes = [astNode].concat(_extensionASTNodes);

          return new GraphQLInterfaceType({
            name: name,
            description: description,
            interfaces: function interfaces() {
              return buildInterfaces(_allNodes);
            },
            fields: function fields() {
              return buildFieldMap(_allNodes);
            },
            astNode: astNode,
            extensionASTNodes: _extensionASTNodes
          });
        }

      case Kind.ENUM_TYPE_DEFINITION:
        {
          var _extensionASTNodes2 = extensionNodes;

          var _allNodes2 = [astNode].concat(_extensionASTNodes2);

          return new GraphQLEnumType({
            name: name,
            description: description,
            values: buildEnumValueMap(_allNodes2),
            astNode: astNode,
            extensionASTNodes: _extensionASTNodes2
          });
        }

      case Kind.UNION_TYPE_DEFINITION:
        {
          var _extensionASTNodes3 = extensionNodes;

          var _allNodes3 = [astNode].concat(_extensionASTNodes3);

          return new GraphQLUnionType({
            name: name,
            description: description,
            types: function types() {
              return buildUnionTypes(_allNodes3);
            },
            astNode: astNode,
            extensionASTNodes: _extensionASTNodes3
          });
        }

      case Kind.SCALAR_TYPE_DEFINITION:
        {
          var _extensionASTNodes4 = extensionNodes;
          return new GraphQLScalarType({
            name: name,
            description: description,
            specifiedByUrl: getSpecifiedByUrl(astNode),
            astNode: astNode,
            extensionASTNodes: _extensionASTNodes4
          });
        }

      case Kind.INPUT_OBJECT_TYPE_DEFINITION:
        {
          var _extensionASTNodes5 = extensionNodes;

          var _allNodes4 = [astNode].concat(_extensionASTNodes5);

          return new GraphQLInputObjectType({
            name: name,
            description: description,
            fields: function fields() {
              return buildInputFieldMap(_allNodes4);
            },
            astNode: astNode,
            extensionASTNodes: _extensionASTNodes5
          });
        }
    } // istanbul ignore next (Not reachable. All possible type definition nodes have been considered)


    false || invariant(0, 'Unexpected type definition node: ' + inspect(astNode));
  }
}
var stdTypeMap = keyMap(specifiedScalarTypes.concat(introspectionTypes), function (type) {
  return type.name;
});
/**
 * Given a field or enum value node, returns the string value for the
 * deprecation reason.
 */

function getDeprecationReason(node) {
  var deprecated = getDirectiveValues(GraphQLDeprecatedDirective, node);
  return deprecated === null || deprecated === void 0 ? void 0 : deprecated.reason;
}
/**
 * Given a scalar node, returns the string value for the specifiedByUrl.
 */


function getSpecifiedByUrl(node) {
  var specifiedBy = getDirectiveValues(GraphQLSpecifiedByDirective, node);
  return specifiedBy === null || specifiedBy === void 0 ? void 0 : specifiedBy.url;
}
/**
 * Given an ast node, returns its string description.
 * @deprecated: provided to ease adoption and will be removed in v16.
 *
 * Accepts options as a second argument:
 *
 *    - commentDescriptions:
 *        Provide true to use preceding comments as the description.
 *
 */


export function getDescription(node, options) {
  if (node.description) {
    return node.description.value;
  }

  if ((options === null || options === void 0 ? void 0 : options.commentDescriptions) === true) {
    var rawValue = getLeadingCommentBlock(node);

    if (rawValue !== undefined) {
      return dedentBlockStringValue('\n' + rawValue);
    }
  }
}

function getLeadingCommentBlock(node) {
  var loc = node.loc;

  if (!loc) {
    return;
  }

  var comments = [];
  var token = loc.startToken.prev;

  while (token != null && token.kind === TokenKind.COMMENT && token.next && token.prev && token.line + 1 === token.next.line && token.line !== token.prev.line) {
    var value = String(token.value);
    comments.push(value);
    token = token.prev;
  }

  return comments.length > 0 ? comments.reverse().join('\n') : undefined;
}
