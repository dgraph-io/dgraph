import objectValues from "../polyfills/objectValues.mjs";
import inspect from "../jsutils/inspect.mjs";
import invariant from "../jsutils/invariant.mjs";
import { print } from "../language/printer.mjs";
import { DirectiveLocation } from "../language/directiveLocation.mjs";
import { astFromValue } from "../utilities/astFromValue.mjs";
import { GraphQLString, GraphQLBoolean } from "./scalars.mjs";
import { GraphQLObjectType, GraphQLEnumType, GraphQLList, GraphQLNonNull, isScalarType, isObjectType, isInterfaceType, isUnionType, isEnumType, isInputObjectType, isListType, isNonNullType, isAbstractType } from "./definition.mjs";
export var __Schema = new GraphQLObjectType({
  name: '__Schema',
  description: 'A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all available types and directives on the server, as well as the entry points for query, mutation, and subscription operations.',
  fields: function fields() {
    return {
      description: {
        type: GraphQLString,
        resolve: function resolve(schema) {
          return schema.description;
        }
      },
      types: {
        description: 'A list of all types supported by this server.',
        type: GraphQLNonNull(GraphQLList(GraphQLNonNull(__Type))),
        resolve: function resolve(schema) {
          return objectValues(schema.getTypeMap());
        }
      },
      queryType: {
        description: 'The type that query operations will be rooted at.',
        type: GraphQLNonNull(__Type),
        resolve: function resolve(schema) {
          return schema.getQueryType();
        }
      },
      mutationType: {
        description: 'If this server supports mutation, the type that mutation operations will be rooted at.',
        type: __Type,
        resolve: function resolve(schema) {
          return schema.getMutationType();
        }
      },
      subscriptionType: {
        description: 'If this server support subscription, the type that subscription operations will be rooted at.',
        type: __Type,
        resolve: function resolve(schema) {
          return schema.getSubscriptionType();
        }
      },
      directives: {
        description: 'A list of all directives supported by this server.',
        type: GraphQLNonNull(GraphQLList(GraphQLNonNull(__Directive))),
        resolve: function resolve(schema) {
          return schema.getDirectives();
        }
      }
    };
  }
});
export var __Directive = new GraphQLObjectType({
  name: '__Directive',
  description: "A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.\n\nIn some cases, you need to provide options to alter GraphQL's execution behavior in ways field arguments will not suffice, such as conditionally including or skipping a field. Directives provide this by describing additional information to the executor.",
  fields: function fields() {
    return {
      name: {
        type: GraphQLNonNull(GraphQLString),
        resolve: function resolve(directive) {
          return directive.name;
        }
      },
      description: {
        type: GraphQLString,
        resolve: function resolve(directive) {
          return directive.description;
        }
      },
      isRepeatable: {
        type: GraphQLNonNull(GraphQLBoolean),
        resolve: function resolve(directive) {
          return directive.isRepeatable;
        }
      },
      locations: {
        type: GraphQLNonNull(GraphQLList(GraphQLNonNull(__DirectiveLocation))),
        resolve: function resolve(directive) {
          return directive.locations;
        }
      },
      args: {
        type: GraphQLNonNull(GraphQLList(GraphQLNonNull(__InputValue))),
        resolve: function resolve(directive) {
          return directive.args;
        }
      }
    };
  }
});
export var __DirectiveLocation = new GraphQLEnumType({
  name: '__DirectiveLocation',
  description: 'A Directive can be adjacent to many parts of the GraphQL language, a __DirectiveLocation describes one such possible adjacencies.',
  values: {
    QUERY: {
      value: DirectiveLocation.QUERY,
      description: 'Location adjacent to a query operation.'
    },
    MUTATION: {
      value: DirectiveLocation.MUTATION,
      description: 'Location adjacent to a mutation operation.'
    },
    SUBSCRIPTION: {
      value: DirectiveLocation.SUBSCRIPTION,
      description: 'Location adjacent to a subscription operation.'
    },
    FIELD: {
      value: DirectiveLocation.FIELD,
      description: 'Location adjacent to a field.'
    },
    FRAGMENT_DEFINITION: {
      value: DirectiveLocation.FRAGMENT_DEFINITION,
      description: 'Location adjacent to a fragment definition.'
    },
    FRAGMENT_SPREAD: {
      value: DirectiveLocation.FRAGMENT_SPREAD,
      description: 'Location adjacent to a fragment spread.'
    },
    INLINE_FRAGMENT: {
      value: DirectiveLocation.INLINE_FRAGMENT,
      description: 'Location adjacent to an inline fragment.'
    },
    VARIABLE_DEFINITION: {
      value: DirectiveLocation.VARIABLE_DEFINITION,
      description: 'Location adjacent to a variable definition.'
    },
    SCHEMA: {
      value: DirectiveLocation.SCHEMA,
      description: 'Location adjacent to a schema definition.'
    },
    SCALAR: {
      value: DirectiveLocation.SCALAR,
      description: 'Location adjacent to a scalar definition.'
    },
    OBJECT: {
      value: DirectiveLocation.OBJECT,
      description: 'Location adjacent to an object type definition.'
    },
    FIELD_DEFINITION: {
      value: DirectiveLocation.FIELD_DEFINITION,
      description: 'Location adjacent to a field definition.'
    },
    ARGUMENT_DEFINITION: {
      value: DirectiveLocation.ARGUMENT_DEFINITION,
      description: 'Location adjacent to an argument definition.'
    },
    INTERFACE: {
      value: DirectiveLocation.INTERFACE,
      description: 'Location adjacent to an interface definition.'
    },
    UNION: {
      value: DirectiveLocation.UNION,
      description: 'Location adjacent to a union definition.'
    },
    ENUM: {
      value: DirectiveLocation.ENUM,
      description: 'Location adjacent to an enum definition.'
    },
    ENUM_VALUE: {
      value: DirectiveLocation.ENUM_VALUE,
      description: 'Location adjacent to an enum value definition.'
    },
    INPUT_OBJECT: {
      value: DirectiveLocation.INPUT_OBJECT,
      description: 'Location adjacent to an input object type definition.'
    },
    INPUT_FIELD_DEFINITION: {
      value: DirectiveLocation.INPUT_FIELD_DEFINITION,
      description: 'Location adjacent to an input object field definition.'
    }
  }
});
export var __Type = new GraphQLObjectType({
  name: '__Type',
  description: 'The fundamental unit of any GraphQL Schema is the type. There are many kinds of types in GraphQL as represented by the `__TypeKind` enum.\n\nDepending on the kind of a type, certain fields describe information about that type. Scalar types provide no information beyond a name, description and optional `specifiedByUrl`, while Enum types provide their values. Object and Interface types provide the fields they describe. Abstract types, Union and Interface, provide the Object types possible at runtime. List and NonNull types compose other types.',
  fields: function fields() {
    return {
      kind: {
        type: GraphQLNonNull(__TypeKind),
        resolve: function resolve(type) {
          if (isScalarType(type)) {
            return TypeKind.SCALAR;
          }

          if (isObjectType(type)) {
            return TypeKind.OBJECT;
          }

          if (isInterfaceType(type)) {
            return TypeKind.INTERFACE;
          }

          if (isUnionType(type)) {
            return TypeKind.UNION;
          }

          if (isEnumType(type)) {
            return TypeKind.ENUM;
          }

          if (isInputObjectType(type)) {
            return TypeKind.INPUT_OBJECT;
          }

          if (isListType(type)) {
            return TypeKind.LIST;
          } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


          if (isNonNullType(type)) {
            return TypeKind.NON_NULL;
          } // istanbul ignore next (Not reachable. All possible types have been considered)


          false || invariant(0, "Unexpected type: \"".concat(inspect(type), "\"."));
        }
      },
      name: {
        type: GraphQLString,
        resolve: function resolve(type) {
          return type.name !== undefined ? type.name : undefined;
        }
      },
      description: {
        type: GraphQLString,
        resolve: function resolve(type) {
          return type.description !== undefined ? type.description : undefined;
        }
      },
      specifiedByUrl: {
        type: GraphQLString,
        resolve: function resolve(obj) {
          return obj.specifiedByUrl !== undefined ? obj.specifiedByUrl : undefined;
        }
      },
      fields: {
        type: GraphQLList(GraphQLNonNull(__Field)),
        args: {
          includeDeprecated: {
            type: GraphQLBoolean,
            defaultValue: false
          }
        },
        resolve: function resolve(type, _ref) {
          var includeDeprecated = _ref.includeDeprecated;

          if (isObjectType(type) || isInterfaceType(type)) {
            var fields = objectValues(type.getFields());

            if (!includeDeprecated) {
              fields = fields.filter(function (field) {
                return !field.isDeprecated;
              });
            }

            return fields;
          }

          return null;
        }
      },
      interfaces: {
        type: GraphQLList(GraphQLNonNull(__Type)),
        resolve: function resolve(type) {
          if (isObjectType(type) || isInterfaceType(type)) {
            return type.getInterfaces();
          }
        }
      },
      possibleTypes: {
        type: GraphQLList(GraphQLNonNull(__Type)),
        resolve: function resolve(type, _args, _context, _ref2) {
          var schema = _ref2.schema;

          if (isAbstractType(type)) {
            return schema.getPossibleTypes(type);
          }
        }
      },
      enumValues: {
        type: GraphQLList(GraphQLNonNull(__EnumValue)),
        args: {
          includeDeprecated: {
            type: GraphQLBoolean,
            defaultValue: false
          }
        },
        resolve: function resolve(type, _ref3) {
          var includeDeprecated = _ref3.includeDeprecated;

          if (isEnumType(type)) {
            var values = type.getValues();

            if (!includeDeprecated) {
              values = values.filter(function (value) {
                return !value.isDeprecated;
              });
            }

            return values;
          }
        }
      },
      inputFields: {
        type: GraphQLList(GraphQLNonNull(__InputValue)),
        resolve: function resolve(type) {
          if (isInputObjectType(type)) {
            return objectValues(type.getFields());
          }
        }
      },
      ofType: {
        type: __Type,
        resolve: function resolve(type) {
          return type.ofType !== undefined ? type.ofType : undefined;
        }
      }
    };
  }
});
export var __Field = new GraphQLObjectType({
  name: '__Field',
  description: 'Object and Interface types are described by a list of Fields, each of which has a name, potentially a list of arguments, and a return type.',
  fields: function fields() {
    return {
      name: {
        type: GraphQLNonNull(GraphQLString),
        resolve: function resolve(field) {
          return field.name;
        }
      },
      description: {
        type: GraphQLString,
        resolve: function resolve(field) {
          return field.description;
        }
      },
      args: {
        type: GraphQLNonNull(GraphQLList(GraphQLNonNull(__InputValue))),
        resolve: function resolve(field) {
          return field.args;
        }
      },
      type: {
        type: GraphQLNonNull(__Type),
        resolve: function resolve(field) {
          return field.type;
        }
      },
      isDeprecated: {
        type: GraphQLNonNull(GraphQLBoolean),
        resolve: function resolve(field) {
          return field.isDeprecated;
        }
      },
      deprecationReason: {
        type: GraphQLString,
        resolve: function resolve(field) {
          return field.deprecationReason;
        }
      }
    };
  }
});
export var __InputValue = new GraphQLObjectType({
  name: '__InputValue',
  description: 'Arguments provided to Fields or Directives and the input fields of an InputObject are represented as Input Values which describe their type and optionally a default value.',
  fields: function fields() {
    return {
      name: {
        type: GraphQLNonNull(GraphQLString),
        resolve: function resolve(inputValue) {
          return inputValue.name;
        }
      },
      description: {
        type: GraphQLString,
        resolve: function resolve(inputValue) {
          return inputValue.description;
        }
      },
      type: {
        type: GraphQLNonNull(__Type),
        resolve: function resolve(inputValue) {
          return inputValue.type;
        }
      },
      defaultValue: {
        type: GraphQLString,
        description: 'A GraphQL-formatted string representing the default value for this input value.',
        resolve: function resolve(inputValue) {
          var type = inputValue.type,
              defaultValue = inputValue.defaultValue;
          var valueAST = astFromValue(defaultValue, type);
          return valueAST ? print(valueAST) : null;
        }
      }
    };
  }
});
export var __EnumValue = new GraphQLObjectType({
  name: '__EnumValue',
  description: 'One possible value for a given Enum. Enum values are unique values, not a placeholder for a string or numeric value. However an Enum value is returned in a JSON response as a string.',
  fields: function fields() {
    return {
      name: {
        type: GraphQLNonNull(GraphQLString),
        resolve: function resolve(enumValue) {
          return enumValue.name;
        }
      },
      description: {
        type: GraphQLString,
        resolve: function resolve(enumValue) {
          return enumValue.description;
        }
      },
      isDeprecated: {
        type: GraphQLNonNull(GraphQLBoolean),
        resolve: function resolve(enumValue) {
          return enumValue.isDeprecated;
        }
      },
      deprecationReason: {
        type: GraphQLString,
        resolve: function resolve(enumValue) {
          return enumValue.deprecationReason;
        }
      }
    };
  }
});
export var TypeKind = Object.freeze({
  SCALAR: 'SCALAR',
  OBJECT: 'OBJECT',
  INTERFACE: 'INTERFACE',
  UNION: 'UNION',
  ENUM: 'ENUM',
  INPUT_OBJECT: 'INPUT_OBJECT',
  LIST: 'LIST',
  NON_NULL: 'NON_NULL'
});
export var __TypeKind = new GraphQLEnumType({
  name: '__TypeKind',
  description: 'An enum describing what kind of type a given `__Type` is.',
  values: {
    SCALAR: {
      value: TypeKind.SCALAR,
      description: 'Indicates this type is a scalar.'
    },
    OBJECT: {
      value: TypeKind.OBJECT,
      description: 'Indicates this type is an object. `fields` and `interfaces` are valid fields.'
    },
    INTERFACE: {
      value: TypeKind.INTERFACE,
      description: 'Indicates this type is an interface. `fields`, `interfaces`, and `possibleTypes` are valid fields.'
    },
    UNION: {
      value: TypeKind.UNION,
      description: 'Indicates this type is a union. `possibleTypes` is a valid field.'
    },
    ENUM: {
      value: TypeKind.ENUM,
      description: 'Indicates this type is an enum. `enumValues` is a valid field.'
    },
    INPUT_OBJECT: {
      value: TypeKind.INPUT_OBJECT,
      description: 'Indicates this type is an input object. `inputFields` is a valid field.'
    },
    LIST: {
      value: TypeKind.LIST,
      description: 'Indicates this type is a list. `ofType` is a valid field.'
    },
    NON_NULL: {
      value: TypeKind.NON_NULL,
      description: 'Indicates this type is a non-null. `ofType` is a valid field.'
    }
  }
});
/**
 * Note that these are GraphQLField and not GraphQLFieldConfig,
 * so the format for args is different.
 */

export var SchemaMetaFieldDef = {
  name: '__schema',
  type: GraphQLNonNull(__Schema),
  description: 'Access the current type schema of this server.',
  args: [],
  resolve: function resolve(_source, _args, _context, _ref4) {
    var schema = _ref4.schema;
    return schema;
  },
  isDeprecated: false,
  deprecationReason: undefined,
  extensions: undefined,
  astNode: undefined
};
export var TypeMetaFieldDef = {
  name: '__type',
  type: __Type,
  description: 'Request the type information of a single type.',
  args: [{
    name: 'name',
    description: undefined,
    type: GraphQLNonNull(GraphQLString),
    defaultValue: undefined,
    extensions: undefined,
    astNode: undefined
  }],
  resolve: function resolve(_source, _ref5, _context, _ref6) {
    var name = _ref5.name;
    var schema = _ref6.schema;
    return schema.getType(name);
  },
  isDeprecated: false,
  deprecationReason: undefined,
  extensions: undefined,
  astNode: undefined
};
export var TypeNameMetaFieldDef = {
  name: '__typename',
  type: GraphQLNonNull(GraphQLString),
  description: 'The name of the current Object type at runtime.',
  args: [],
  resolve: function resolve(_source, _args, _context, _ref7) {
    var parentType = _ref7.parentType;
    return parentType.name;
  },
  isDeprecated: false,
  deprecationReason: undefined,
  extensions: undefined,
  astNode: undefined
};
export var introspectionTypes = Object.freeze([__Schema, __Directive, __DirectiveLocation, __Type, __Field, __InputValue, __EnumValue, __TypeKind]);
export function isIntrospectionType(type) {
  return introspectionTypes.some(function (_ref8) {
    var name = _ref8.name;
    return type.name === name;
  });
}
