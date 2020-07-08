// @flow strict

import objectEntries from '../polyfills/objectEntries';
import { SYMBOL_TO_STRING_TAG } from '../polyfills/symbols';

import type { Path } from '../jsutils/Path';
import type { PromiseOrValue } from '../jsutils/PromiseOrValue';
import type {
  ObjMap,
  ReadOnlyObjMap,
  ReadOnlyObjMapLike,
} from '../jsutils/ObjMap';
import inspect from '../jsutils/inspect';
import keyMap from '../jsutils/keyMap';
import mapValue from '../jsutils/mapValue';
import toObjMap from '../jsutils/toObjMap';
import devAssert from '../jsutils/devAssert';
import keyValMap from '../jsutils/keyValMap';
import instanceOf from '../jsutils/instanceOf';
import didYouMean from '../jsutils/didYouMean';
import isObjectLike from '../jsutils/isObjectLike';
import identityFunc from '../jsutils/identityFunc';
import defineInspect from '../jsutils/defineInspect';
import suggestionList from '../jsutils/suggestionList';

import { GraphQLError } from '../error/GraphQLError';

import { Kind } from '../language/kinds';
import { print } from '../language/printer';
import type {
  ScalarTypeDefinitionNode,
  ObjectTypeDefinitionNode,
  FieldDefinitionNode,
  InputValueDefinitionNode,
  InterfaceTypeDefinitionNode,
  UnionTypeDefinitionNode,
  EnumTypeDefinitionNode,
  EnumValueDefinitionNode,
  InputObjectTypeDefinitionNode,
  ScalarTypeExtensionNode,
  ObjectTypeExtensionNode,
  InterfaceTypeExtensionNode,
  UnionTypeExtensionNode,
  EnumTypeExtensionNode,
  InputObjectTypeExtensionNode,
  OperationDefinitionNode,
  FieldNode,
  FragmentDefinitionNode,
  ValueNode,
} from '../language/ast';

import { valueFromASTUntyped } from '../utilities/valueFromASTUntyped';

import type { GraphQLSchema } from './schema';

// Predicates & Assertions

/**
 * These are all of the possible kinds of types.
 */
export type GraphQLType =
  | GraphQLScalarType
  | GraphQLObjectType
  | GraphQLInterfaceType
  | GraphQLUnionType
  | GraphQLEnumType
  | GraphQLInputObjectType
  | GraphQLList<any>
  | GraphQLNonNull<any>;

export function isType(type: mixed): boolean %checks {
  return (
    isScalarType(type) ||
    isObjectType(type) ||
    isInterfaceType(type) ||
    isUnionType(type) ||
    isEnumType(type) ||
    isInputObjectType(type) ||
    isListType(type) ||
    isNonNullType(type)
  );
}

export function assertType(type: mixed): GraphQLType {
  if (!isType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL type.`);
  }
  return type;
}

/**
 * There are predicates for each kind of GraphQL type.
 */

declare function isScalarType(type: mixed): boolean %checks(type instanceof
  GraphQLScalarType);
// eslint-disable-next-line no-redeclare
export function isScalarType(type) {
  return instanceOf(type, GraphQLScalarType);
}

export function assertScalarType(type: mixed): GraphQLScalarType {
  if (!isScalarType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL Scalar type.`);
  }
  return type;
}

declare function isObjectType(type: mixed): boolean %checks(type instanceof
  GraphQLObjectType);
// eslint-disable-next-line no-redeclare
export function isObjectType(type) {
  return instanceOf(type, GraphQLObjectType);
}

export function assertObjectType(type: mixed): GraphQLObjectType {
  if (!isObjectType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL Object type.`);
  }
  return type;
}

declare function isInterfaceType(type: mixed): boolean %checks(type instanceof
  GraphQLInterfaceType);
// eslint-disable-next-line no-redeclare
export function isInterfaceType(type) {
  return instanceOf(type, GraphQLInterfaceType);
}

export function assertInterfaceType(type: mixed): GraphQLInterfaceType {
  if (!isInterfaceType(type)) {
    throw new Error(
      `Expected ${inspect(type)} to be a GraphQL Interface type.`,
    );
  }
  return type;
}

declare function isUnionType(type: mixed): boolean %checks(type instanceof
  GraphQLUnionType);
// eslint-disable-next-line no-redeclare
export function isUnionType(type) {
  return instanceOf(type, GraphQLUnionType);
}

export function assertUnionType(type: mixed): GraphQLUnionType {
  if (!isUnionType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL Union type.`);
  }
  return type;
}

declare function isEnumType(type: mixed): boolean %checks(type instanceof
  GraphQLEnumType);
// eslint-disable-next-line no-redeclare
export function isEnumType(type) {
  return instanceOf(type, GraphQLEnumType);
}

export function assertEnumType(type: mixed): GraphQLEnumType {
  if (!isEnumType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL Enum type.`);
  }
  return type;
}

declare function isInputObjectType(type: mixed): boolean %checks(type instanceof
  GraphQLInputObjectType);
// eslint-disable-next-line no-redeclare
export function isInputObjectType(type) {
  return instanceOf(type, GraphQLInputObjectType);
}

export function assertInputObjectType(type: mixed): GraphQLInputObjectType {
  if (!isInputObjectType(type)) {
    throw new Error(
      `Expected ${inspect(type)} to be a GraphQL Input Object type.`,
    );
  }
  return type;
}

declare function isListType(type: mixed): boolean %checks(type instanceof
  GraphQLList);
// eslint-disable-next-line no-redeclare
export function isListType(type) {
  return instanceOf(type, GraphQLList);
}

export function assertListType(type: mixed): GraphQLList<any> {
  if (!isListType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL List type.`);
  }
  return type;
}

declare function isNonNullType(type: mixed): boolean %checks(type instanceof
  GraphQLNonNull);
// eslint-disable-next-line no-redeclare
export function isNonNullType(type) {
  return instanceOf(type, GraphQLNonNull);
}

export function assertNonNullType(type: mixed): GraphQLNonNull<any> {
  if (!isNonNullType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL Non-Null type.`);
  }
  return type;
}

/**
 * These types may be used as input types for arguments and directives.
 */
export type GraphQLInputType =
  | GraphQLScalarType
  | GraphQLEnumType
  | GraphQLInputObjectType
  | GraphQLList<GraphQLInputType>
  | GraphQLNonNull<
      | GraphQLScalarType
      | GraphQLEnumType
      | GraphQLInputObjectType
      | GraphQLList<GraphQLInputType>,
    >;

export function isInputType(type: mixed): boolean %checks {
  return (
    isScalarType(type) ||
    isEnumType(type) ||
    isInputObjectType(type) ||
    (isWrappingType(type) && isInputType(type.ofType))
  );
}

export function assertInputType(type: mixed): GraphQLInputType {
  if (!isInputType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL input type.`);
  }
  return type;
}

/**
 * These types may be used as output types as the result of fields.
 */
export type GraphQLOutputType =
  | GraphQLScalarType
  | GraphQLObjectType
  | GraphQLInterfaceType
  | GraphQLUnionType
  | GraphQLEnumType
  | GraphQLList<GraphQLOutputType>
  | GraphQLNonNull<
      | GraphQLScalarType
      | GraphQLObjectType
      | GraphQLInterfaceType
      | GraphQLUnionType
      | GraphQLEnumType
      | GraphQLList<GraphQLOutputType>,
    >;

export function isOutputType(type: mixed): boolean %checks {
  return (
    isScalarType(type) ||
    isObjectType(type) ||
    isInterfaceType(type) ||
    isUnionType(type) ||
    isEnumType(type) ||
    (isWrappingType(type) && isOutputType(type.ofType))
  );
}

export function assertOutputType(type: mixed): GraphQLOutputType {
  if (!isOutputType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL output type.`);
  }
  return type;
}

/**
 * These types may describe types which may be leaf values.
 */
export type GraphQLLeafType = GraphQLScalarType | GraphQLEnumType;

export function isLeafType(type: mixed): boolean %checks {
  return isScalarType(type) || isEnumType(type);
}

export function assertLeafType(type: mixed): GraphQLLeafType {
  if (!isLeafType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL leaf type.`);
  }
  return type;
}

/**
 * These types may describe the parent context of a selection set.
 */
export type GraphQLCompositeType =
  | GraphQLObjectType
  | GraphQLInterfaceType
  | GraphQLUnionType;

export function isCompositeType(type: mixed): boolean %checks {
  return isObjectType(type) || isInterfaceType(type) || isUnionType(type);
}

export function assertCompositeType(type: mixed): GraphQLCompositeType {
  if (!isCompositeType(type)) {
    throw new Error(
      `Expected ${inspect(type)} to be a GraphQL composite type.`,
    );
  }
  return type;
}

/**
 * These types may describe the parent context of a selection set.
 */
export type GraphQLAbstractType = GraphQLInterfaceType | GraphQLUnionType;

export function isAbstractType(type: mixed): boolean %checks {
  return isInterfaceType(type) || isUnionType(type);
}

export function assertAbstractType(type: mixed): GraphQLAbstractType {
  if (!isAbstractType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL abstract type.`);
  }
  return type;
}

/**
 * List Type Wrapper
 *
 * A list is a wrapping type which points to another type.
 * Lists are often created within the context of defining the fields of
 * an object type.
 *
 * Example:
 *
 *     const PersonType = new GraphQLObjectType({
 *       name: 'Person',
 *       fields: () => ({
 *         parents: { type: GraphQLList(PersonType) },
 *         children: { type: GraphQLList(PersonType) },
 *       })
 *     })
 *
 */
// FIXME: workaround to fix issue with Babel parser
/* ::
declare class GraphQLList<+T: GraphQLType> {
  +ofType: T;
  static <T>(ofType: T): GraphQLList<T>;
  // Note: constructors cannot be used for covariant types. Drop the "new".
  constructor(ofType: GraphQLType): void;
}
*/

export function GraphQLList(ofType) {
  if (this instanceof GraphQLList) {
    this.ofType = assertType(ofType);
  } else {
    return new GraphQLList(ofType);
  }
}

// Need to cast through any to alter the prototype.
(GraphQLList.prototype: any).toString = function toString() {
  return '[' + String(this.ofType) + ']';
};

(GraphQLList.prototype: any).toJSON = function toJSON() {
  return this.toString();
};

Object.defineProperty(GraphQLList.prototype, SYMBOL_TO_STRING_TAG, {
  get() {
    return 'GraphQLList';
  },
});

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLList);

/**
 * Non-Null Type Wrapper
 *
 * A non-null is a wrapping type which points to another type.
 * Non-null types enforce that their values are never null and can ensure
 * an error is raised if this ever occurs during a request. It is useful for
 * fields which you can make a strong guarantee on non-nullability, for example
 * usually the id field of a database row will never be null.
 *
 * Example:
 *
 *     const RowType = new GraphQLObjectType({
 *       name: 'Row',
 *       fields: () => ({
 *         id: { type: GraphQLNonNull(GraphQLString) },
 *       })
 *     })
 *
 * Note: the enforcement of non-nullability occurs within the executor.
 */
// FIXME: workaround to fix issue with Babel parser
/* ::
declare class GraphQLNonNull<+T: GraphQLNullableType> {
  +ofType: T;
  static <T>(ofType: T): GraphQLNonNull<T>;
  // Note: constructors cannot be used for covariant types. Drop the "new".
  constructor(ofType: GraphQLType): void;
}
*/

export function GraphQLNonNull(ofType) {
  if (this instanceof GraphQLNonNull) {
    this.ofType = assertNullableType(ofType);
  } else {
    return new GraphQLNonNull(ofType);
  }
}

// Need to cast through any to alter the prototype.
(GraphQLNonNull.prototype: any).toString = function toString() {
  return String(this.ofType) + '!';
};

(GraphQLNonNull.prototype: any).toJSON = function toJSON() {
  return this.toString();
};

Object.defineProperty(GraphQLNonNull.prototype, SYMBOL_TO_STRING_TAG, {
  get() {
    return 'GraphQLNonNull';
  },
});

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLNonNull);

/**
 * These types wrap and modify other types
 */

export type GraphQLWrappingType = GraphQLList<any> | GraphQLNonNull<any>;

export function isWrappingType(type: mixed): boolean %checks {
  return isListType(type) || isNonNullType(type);
}

export function assertWrappingType(type: mixed): GraphQLWrappingType {
  if (!isWrappingType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL wrapping type.`);
  }
  return type;
}

/**
 * These types can all accept null as a value.
 */
export type GraphQLNullableType =
  | GraphQLScalarType
  | GraphQLObjectType
  | GraphQLInterfaceType
  | GraphQLUnionType
  | GraphQLEnumType
  | GraphQLInputObjectType
  | GraphQLList<any>;

export function isNullableType(type: mixed): boolean %checks {
  return isType(type) && !isNonNullType(type);
}

export function assertNullableType(type: mixed): GraphQLNullableType {
  if (!isNullableType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL nullable type.`);
  }
  return type;
}

/* eslint-disable no-redeclare */
declare function getNullableType(type: void | null): void;
declare function getNullableType<T: GraphQLNullableType>(type: T): T;
declare function getNullableType<T>(type: GraphQLNonNull<T>): T;
export function getNullableType(type) {
  /* eslint-enable no-redeclare */
  if (type) {
    return isNonNullType(type) ? type.ofType : type;
  }
}

/**
 * These named types do not include modifiers like List or NonNull.
 */
export type GraphQLNamedType =
  | GraphQLScalarType
  | GraphQLObjectType
  | GraphQLInterfaceType
  | GraphQLUnionType
  | GraphQLEnumType
  | GraphQLInputObjectType;

export function isNamedType(type: mixed): boolean %checks {
  return (
    isScalarType(type) ||
    isObjectType(type) ||
    isInterfaceType(type) ||
    isUnionType(type) ||
    isEnumType(type) ||
    isInputObjectType(type)
  );
}

export function assertNamedType(type: mixed): GraphQLNamedType {
  if (!isNamedType(type)) {
    throw new Error(`Expected ${inspect(type)} to be a GraphQL named type.`);
  }
  return type;
}

/* eslint-disable no-redeclare */
declare function getNamedType(type: void | null): void;
declare function getNamedType(type: GraphQLType): GraphQLNamedType;
export function getNamedType(type) {
  /* eslint-enable no-redeclare */
  if (type) {
    let unwrappedType = type;
    while (isWrappingType(unwrappedType)) {
      unwrappedType = unwrappedType.ofType;
    }
    return unwrappedType;
  }
}

/**
 * Used while defining GraphQL types to allow for circular references in
 * otherwise immutable type definitions.
 */
export type Thunk<+T> = (() => T) | T;

function resolveThunk<+T>(thunk: Thunk<T>): T {
  // $FlowFixMe(>=0.90.0)
  return typeof thunk === 'function' ? thunk() : thunk;
}

function undefineIfEmpty<T>(arr: ?$ReadOnlyArray<T>): ?$ReadOnlyArray<T> {
  return arr && arr.length > 0 ? arr : undefined;
}

/**
 * Scalar Type Definition
 *
 * The leaf values of any request and input values to arguments are
 * Scalars (or Enums) and are defined with a name and a series of functions
 * used to parse input from ast or variables and to ensure validity.
 *
 * If a type's serialize function does not return a value (i.e. it returns
 * `undefined`) then an error will be raised and a `null` value will be returned
 * in the response. If the serialize function returns `null`, then no error will
 * be included in the response.
 *
 * Example:
 *
 *     const OddType = new GraphQLScalarType({
 *       name: 'Odd',
 *       serialize(value) {
 *         if (value % 2 === 1) {
 *           return value;
 *         }
 *       }
 *     });
 *
 */
export class GraphQLScalarType {
  name: string;
  description: ?string;
  specifiedByUrl: ?string;
  serialize: GraphQLScalarSerializer<mixed>;
  parseValue: GraphQLScalarValueParser<mixed>;
  parseLiteral: GraphQLScalarLiteralParser<mixed>;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?ScalarTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<ScalarTypeExtensionNode>;

  constructor(config: $ReadOnly<GraphQLScalarTypeConfig<mixed, mixed>>): void {
    const parseValue = config.parseValue ?? identityFunc;
    this.name = config.name;
    this.description = config.description;
    this.specifiedByUrl = config.specifiedByUrl;
    this.serialize = config.serialize ?? identityFunc;
    this.parseValue = parseValue;
    this.parseLiteral =
      config.parseLiteral ?? ((node) => parseValue(valueFromASTUntyped(node)));
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    devAssert(typeof config.name === 'string', 'Must provide name.');

    devAssert(
      config.specifiedByUrl == null ||
        typeof config.specifiedByUrl === 'string',
      `${this.name} must provide "specifiedByUrl" as a string, ` +
        `but got: ${inspect(config.specifiedByUrl)}.`,
    );

    devAssert(
      config.serialize == null || typeof config.serialize === 'function',
      `${this.name} must provide "serialize" function. If this custom Scalar is also used as an input type, ensure "parseValue" and "parseLiteral" functions are also provided.`,
    );

    if (config.parseLiteral) {
      devAssert(
        typeof config.parseValue === 'function' &&
          typeof config.parseLiteral === 'function',
        `${this.name} must provide both "parseValue" and "parseLiteral" functions.`,
      );
    }
  }

  toConfig(): {|
    ...GraphQLScalarTypeConfig<mixed, mixed>,
    serialize: GraphQLScalarSerializer<mixed>,
    parseValue: GraphQLScalarValueParser<mixed>,
    parseLiteral: GraphQLScalarLiteralParser<mixed>,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<ScalarTypeExtensionNode>,
  |} {
    return {
      name: this.name,
      description: this.description,
      specifiedByUrl: this.specifiedByUrl,
      serialize: this.serialize,
      parseValue: this.parseValue,
      parseLiteral: this.parseLiteral,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes ?? [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLScalarType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLScalarType);

export type GraphQLScalarSerializer<TExternal> = (
  outputValue: mixed,
) => ?TExternal;

export type GraphQLScalarValueParser<TInternal> = (
  inputValue: mixed,
) => ?TInternal;

export type GraphQLScalarLiteralParser<TInternal> = (
  valueNode: ValueNode,
  variables: ?ObjMap<mixed>,
) => ?TInternal;

export type GraphQLScalarTypeConfig<TInternal, TExternal> = {|
  name: string,
  description?: ?string,
  specifiedByUrl?: ?string,
  // Serializes an internal value to include in a response.
  serialize?: GraphQLScalarSerializer<TExternal>,
  // Parses an externally provided value to use as an input.
  parseValue?: GraphQLScalarValueParser<TInternal>,
  // Parses an externally provided literal value to use as an input.
  parseLiteral?: GraphQLScalarLiteralParser<TInternal>,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?ScalarTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<ScalarTypeExtensionNode>,
|};

/**
 * Object Type Definition
 *
 * Almost all of the GraphQL types you define will be object types. Object types
 * have a name, but most importantly describe their fields.
 *
 * Example:
 *
 *     const AddressType = new GraphQLObjectType({
 *       name: 'Address',
 *       fields: {
 *         street: { type: GraphQLString },
 *         number: { type: GraphQLInt },
 *         formatted: {
 *           type: GraphQLString,
 *           resolve(obj) {
 *             return obj.number + ' ' + obj.street
 *           }
 *         }
 *       }
 *     });
 *
 * When two types need to refer to each other, or a type needs to refer to
 * itself in a field, you can use a function expression (aka a closure or a
 * thunk) to supply the fields lazily.
 *
 * Example:
 *
 *     const PersonType = new GraphQLObjectType({
 *       name: 'Person',
 *       fields: () => ({
 *         name: { type: GraphQLString },
 *         bestFriend: { type: PersonType },
 *       })
 *     });
 *
 */
export class GraphQLObjectType {
  name: string;
  description: ?string;
  isTypeOf: ?GraphQLIsTypeOfFn<any, any>;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?ObjectTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<ObjectTypeExtensionNode>;

  _fields: Thunk<GraphQLFieldMap<any, any>>;
  _interfaces: Thunk<Array<GraphQLInterfaceType>>;

  constructor(config: $ReadOnly<GraphQLObjectTypeConfig<any, any>>): void {
    this.name = config.name;
    this.description = config.description;
    this.isTypeOf = config.isTypeOf;
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    this._fields = defineFieldMap.bind(undefined, config);
    this._interfaces = defineInterfaces.bind(undefined, config);
    devAssert(typeof config.name === 'string', 'Must provide name.');
    devAssert(
      config.isTypeOf == null || typeof config.isTypeOf === 'function',
      `${this.name} must provide "isTypeOf" as a function, ` +
        `but got: ${inspect(config.isTypeOf)}.`,
    );
  }

  getFields(): GraphQLFieldMap<any, any> {
    if (typeof this._fields === 'function') {
      this._fields = this._fields();
    }
    return this._fields;
  }

  getInterfaces(): Array<GraphQLInterfaceType> {
    if (typeof this._interfaces === 'function') {
      this._interfaces = this._interfaces();
    }
    return this._interfaces;
  }

  toConfig(): {|
    ...GraphQLObjectTypeConfig<any, any>,
    interfaces: Array<GraphQLInterfaceType>,
    fields: GraphQLFieldConfigMap<any, any>,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<ObjectTypeExtensionNode>,
  |} {
    return {
      name: this.name,
      description: this.description,
      interfaces: this.getInterfaces(),
      fields: fieldsToFieldsConfig(this.getFields()),
      isTypeOf: this.isTypeOf,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes || [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLObjectType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLObjectType);

function defineInterfaces(
  config: $ReadOnly<
    | GraphQLObjectTypeConfig<mixed, mixed>
    | GraphQLInterfaceTypeConfig<mixed, mixed>,
  >,
): Array<GraphQLInterfaceType> {
  const interfaces = resolveThunk(config.interfaces) ?? [];
  devAssert(
    Array.isArray(interfaces),
    `${config.name} interfaces must be an Array or a function which returns an Array.`,
  );
  return interfaces;
}

function defineFieldMap<TSource, TContext>(
  config: $ReadOnly<
    | GraphQLObjectTypeConfig<TSource, TContext>
    | GraphQLInterfaceTypeConfig<TSource, TContext>,
  >,
): GraphQLFieldMap<TSource, TContext> {
  const fieldMap = resolveThunk(config.fields);
  devAssert(
    isPlainObj(fieldMap),
    `${config.name} fields must be an object with field names as keys or a function which returns such an object.`,
  );

  return mapValue(fieldMap, (fieldConfig, fieldName) => {
    devAssert(
      isPlainObj(fieldConfig),
      `${config.name}.${fieldName} field config must be an object.`,
    );
    devAssert(
      !('isDeprecated' in fieldConfig),
      `${config.name}.${fieldName} should provide "deprecationReason" instead of "isDeprecated".`,
    );
    devAssert(
      fieldConfig.resolve == null || typeof fieldConfig.resolve === 'function',
      `${config.name}.${fieldName} field resolver must be a function if ` +
        `provided, but got: ${inspect(fieldConfig.resolve)}.`,
    );

    const argsConfig = fieldConfig.args ?? {};
    devAssert(
      isPlainObj(argsConfig),
      `${config.name}.${fieldName} args must be an object with argument names as keys.`,
    );

    const args = objectEntries(argsConfig).map(([argName, argConfig]) => ({
      name: argName,
      description: argConfig.description,
      type: argConfig.type,
      defaultValue: argConfig.defaultValue,
      extensions: argConfig.extensions && toObjMap(argConfig.extensions),
      astNode: argConfig.astNode,
    }));

    return {
      name: fieldName,
      description: fieldConfig.description,
      type: fieldConfig.type,
      args,
      resolve: fieldConfig.resolve,
      subscribe: fieldConfig.subscribe,
      isDeprecated: fieldConfig.deprecationReason != null,
      deprecationReason: fieldConfig.deprecationReason,
      extensions: fieldConfig.extensions && toObjMap(fieldConfig.extensions),
      astNode: fieldConfig.astNode,
    };
  });
}

function isPlainObj(obj) {
  return isObjectLike(obj) && !Array.isArray(obj);
}

function fieldsToFieldsConfig(fields) {
  return mapValue(fields, (field) => ({
    description: field.description,
    type: field.type,
    args: argsToArgsConfig(field.args),
    resolve: field.resolve,
    subscribe: field.subscribe,
    deprecationReason: field.deprecationReason,
    extensions: field.extensions,
    astNode: field.astNode,
  }));
}

/**
 * @internal
 */
export function argsToArgsConfig(
  args: $ReadOnlyArray<GraphQLArgument>,
): GraphQLFieldConfigArgumentMap {
  return keyValMap(
    args,
    (arg) => arg.name,
    (arg) => ({
      description: arg.description,
      type: arg.type,
      defaultValue: arg.defaultValue,
      extensions: arg.extensions,
      astNode: arg.astNode,
    }),
  );
}

export type GraphQLObjectTypeConfig<TSource, TContext> = {|
  name: string,
  description?: ?string,
  interfaces?: Thunk<?Array<GraphQLInterfaceType>>,
  fields: Thunk<GraphQLFieldConfigMap<TSource, TContext>>,
  isTypeOf?: ?GraphQLIsTypeOfFn<TSource, TContext>,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?ObjectTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<ObjectTypeExtensionNode>,
|};

export type GraphQLTypeResolver<TSource, TContext> = (
  value: TSource,
  context: TContext,
  info: GraphQLResolveInfo,
  abstractType: GraphQLAbstractType,
) => PromiseOrValue<?GraphQLObjectType | string>;

export type GraphQLIsTypeOfFn<TSource, TContext> = (
  source: TSource,
  context: TContext,
  info: GraphQLResolveInfo,
) => PromiseOrValue<boolean>;

export type GraphQLFieldResolver<
  TSource,
  TContext,
  TArgs = { [argument: string]: any, ... },
> = (
  source: TSource,
  args: TArgs,
  context: TContext,
  info: GraphQLResolveInfo,
) => mixed;

export type GraphQLResolveInfo = {|
  +fieldName: string,
  +fieldNodes: $ReadOnlyArray<FieldNode>,
  +returnType: GraphQLOutputType,
  +parentType: GraphQLObjectType,
  +path: Path,
  +schema: GraphQLSchema,
  +fragments: ObjMap<FragmentDefinitionNode>,
  +rootValue: mixed,
  +operation: OperationDefinitionNode,
  +variableValues: { [variable: string]: mixed, ... },
|};

export type GraphQLFieldConfig<
  TSource,
  TContext,
  TArgs = { [argument: string]: any, ... },
> = {|
  description?: ?string,
  type: GraphQLOutputType,
  args?: GraphQLFieldConfigArgumentMap,
  resolve?: GraphQLFieldResolver<TSource, TContext, TArgs>,
  subscribe?: GraphQLFieldResolver<TSource, TContext, TArgs>,
  deprecationReason?: ?string,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?FieldDefinitionNode,
|};

export type GraphQLFieldConfigArgumentMap = ObjMap<GraphQLArgumentConfig>;

export type GraphQLArgumentConfig = {|
  description?: ?string,
  type: GraphQLInputType,
  defaultValue?: mixed,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?InputValueDefinitionNode,
|};

export type GraphQLFieldConfigMap<TSource, TContext> = ObjMap<
  GraphQLFieldConfig<TSource, TContext>,
>;

export type GraphQLField<
  TSource,
  TContext,
  TArgs = { [argument: string]: any, ... },
> = {|
  name: string,
  description: ?string,
  type: GraphQLOutputType,
  args: Array<GraphQLArgument>,
  resolve?: GraphQLFieldResolver<TSource, TContext, TArgs>,
  subscribe?: GraphQLFieldResolver<TSource, TContext, TArgs>,
  isDeprecated: boolean,
  deprecationReason: ?string,
  extensions: ?ReadOnlyObjMap<mixed>,
  astNode: ?FieldDefinitionNode,
|};

export type GraphQLArgument = {|
  name: string,
  description: ?string,
  type: GraphQLInputType,
  defaultValue: mixed,
  extensions: ?ReadOnlyObjMap<mixed>,
  astNode: ?InputValueDefinitionNode,
|};

export function isRequiredArgument(arg: GraphQLArgument): boolean %checks {
  return isNonNullType(arg.type) && arg.defaultValue === undefined;
}

export type GraphQLFieldMap<TSource, TContext> = ObjMap<
  GraphQLField<TSource, TContext>,
>;

/**
 * Interface Type Definition
 *
 * When a field can return one of a heterogeneous set of types, a Interface type
 * is used to describe what types are possible, what fields are in common across
 * all types, as well as a function to determine which type is actually used
 * when the field is resolved.
 *
 * Example:
 *
 *     const EntityType = new GraphQLInterfaceType({
 *       name: 'Entity',
 *       fields: {
 *         name: { type: GraphQLString }
 *       }
 *     });
 *
 */
export class GraphQLInterfaceType {
  name: string;
  description: ?string;
  resolveType: ?GraphQLTypeResolver<any, any>;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?InterfaceTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<InterfaceTypeExtensionNode>;

  _fields: Thunk<GraphQLFieldMap<any, any>>;
  _interfaces: Thunk<Array<GraphQLInterfaceType>>;

  constructor(config: $ReadOnly<GraphQLInterfaceTypeConfig<any, any>>): void {
    this.name = config.name;
    this.description = config.description;
    this.resolveType = config.resolveType;
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    this._fields = defineFieldMap.bind(undefined, config);
    this._interfaces = defineInterfaces.bind(undefined, config);
    devAssert(typeof config.name === 'string', 'Must provide name.');
    devAssert(
      config.resolveType == null || typeof config.resolveType === 'function',
      `${this.name} must provide "resolveType" as a function, ` +
        `but got: ${inspect(config.resolveType)}.`,
    );
  }

  getFields(): GraphQLFieldMap<any, any> {
    if (typeof this._fields === 'function') {
      this._fields = this._fields();
    }
    return this._fields;
  }

  getInterfaces(): Array<GraphQLInterfaceType> {
    if (typeof this._interfaces === 'function') {
      this._interfaces = this._interfaces();
    }
    return this._interfaces;
  }

  toConfig(): {|
    ...GraphQLInterfaceTypeConfig<any, any>,
    interfaces: Array<GraphQLInterfaceType>,
    fields: GraphQLFieldConfigMap<any, any>,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<InterfaceTypeExtensionNode>,
  |} {
    return {
      name: this.name,
      description: this.description,
      interfaces: this.getInterfaces(),
      fields: fieldsToFieldsConfig(this.getFields()),
      resolveType: this.resolveType,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes ?? [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLInterfaceType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLInterfaceType);

export type GraphQLInterfaceTypeConfig<TSource, TContext> = {|
  name: string,
  description?: ?string,
  interfaces?: Thunk<?Array<GraphQLInterfaceType>>,
  fields: Thunk<GraphQLFieldConfigMap<TSource, TContext>>,
  /**
   * Optionally provide a custom type resolver function. If one is not provided,
   * the default implementation will call `isTypeOf` on each implementing
   * Object type.
   */
  resolveType?: ?GraphQLTypeResolver<TSource, TContext>,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?InterfaceTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<InterfaceTypeExtensionNode>,
|};

/**
 * Union Type Definition
 *
 * When a field can return one of a heterogeneous set of types, a Union type
 * is used to describe what types are possible as well as providing a function
 * to determine which type is actually used when the field is resolved.
 *
 * Example:
 *
 *     const PetType = new GraphQLUnionType({
 *       name: 'Pet',
 *       types: [ DogType, CatType ],
 *       resolveType(value) {
 *         if (value instanceof Dog) {
 *           return DogType;
 *         }
 *         if (value instanceof Cat) {
 *           return CatType;
 *         }
 *       }
 *     });
 *
 */
export class GraphQLUnionType {
  name: string;
  description: ?string;
  resolveType: ?GraphQLTypeResolver<any, any>;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?UnionTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<UnionTypeExtensionNode>;

  _types: Thunk<Array<GraphQLObjectType>>;

  constructor(config: $ReadOnly<GraphQLUnionTypeConfig<any, any>>): void {
    this.name = config.name;
    this.description = config.description;
    this.resolveType = config.resolveType;
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    this._types = defineTypes.bind(undefined, config);
    devAssert(typeof config.name === 'string', 'Must provide name.');
    devAssert(
      config.resolveType == null || typeof config.resolveType === 'function',
      `${this.name} must provide "resolveType" as a function, ` +
        `but got: ${inspect(config.resolveType)}.`,
    );
  }

  getTypes(): Array<GraphQLObjectType> {
    if (typeof this._types === 'function') {
      this._types = this._types();
    }
    return this._types;
  }

  toConfig(): {|
    ...GraphQLUnionTypeConfig<any, any>,
    types: Array<GraphQLObjectType>,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<UnionTypeExtensionNode>,
  |} {
    return {
      name: this.name,
      description: this.description,
      types: this.getTypes(),
      resolveType: this.resolveType,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes ?? [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLUnionType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLUnionType);

function defineTypes(
  config: $ReadOnly<GraphQLUnionTypeConfig<mixed, mixed>>,
): Array<GraphQLObjectType> {
  const types = resolveThunk(config.types);
  devAssert(
    Array.isArray(types),
    `Must provide Array of types or a function which returns such an array for Union ${config.name}.`,
  );
  return types;
}

export type GraphQLUnionTypeConfig<TSource, TContext> = {|
  name: string,
  description?: ?string,
  types: Thunk<Array<GraphQLObjectType>>,
  /**
   * Optionally provide a custom type resolver function. If one is not provided,
   * the default implementation will call `isTypeOf` on each implementing
   * Object type.
   */
  resolveType?: ?GraphQLTypeResolver<TSource, TContext>,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?UnionTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<UnionTypeExtensionNode>,
|};

/**
 * Enum Type Definition
 *
 * Some leaf values of requests and input values are Enums. GraphQL serializes
 * Enum values as strings, however internally Enums can be represented by any
 * kind of type, often integers.
 *
 * Example:
 *
 *     const RGBType = new GraphQLEnumType({
 *       name: 'RGB',
 *       values: {
 *         RED: { value: 0 },
 *         GREEN: { value: 1 },
 *         BLUE: { value: 2 }
 *       }
 *     });
 *
 * Note: If a value is not provided in a definition, the name of the enum value
 * will be used as its internal value.
 */
export class GraphQLEnumType /* <T> */ {
  name: string;
  description: ?string;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?EnumTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<EnumTypeExtensionNode>;

  _values: Array<GraphQLEnumValue /* <T> */>;
  _valueLookup: Map<any /* T */, GraphQLEnumValue>;
  _nameLookup: ObjMap<GraphQLEnumValue>;

  constructor(config: $ReadOnly<GraphQLEnumTypeConfig /* <T> */>): void {
    this.name = config.name;
    this.description = config.description;
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    this._values = defineEnumValues(this.name, config.values);
    this._valueLookup = new Map(
      this._values.map((enumValue) => [enumValue.value, enumValue]),
    );
    this._nameLookup = keyMap(this._values, (value) => value.name);

    devAssert(typeof config.name === 'string', 'Must provide name.');
  }

  getValues(): Array<GraphQLEnumValue /* <T> */> {
    return this._values;
  }

  getValue(name: string): ?GraphQLEnumValue {
    return this._nameLookup[name];
  }

  serialize(outputValue: mixed /* T */): ?string {
    const enumValue = this._valueLookup.get(outputValue);
    if (enumValue === undefined) {
      throw new GraphQLError(
        `Enum "${this.name}" cannot represent value: ${inspect(outputValue)}`,
      );
    }
    return enumValue.name;
  }

  parseValue(inputValue: mixed): ?any /* T */ {
    if (typeof inputValue !== 'string') {
      const valueStr = inspect(inputValue);
      throw new GraphQLError(
        `Enum "${this.name}" cannot represent non-string value: ${valueStr}.` +
          didYouMeanEnumValue(this, valueStr),
      );
    }

    const enumValue = this.getValue(inputValue);
    if (enumValue == null) {
      throw new GraphQLError(
        `Value "${inputValue}" does not exist in "${this.name}" enum.` +
          didYouMeanEnumValue(this, inputValue),
      );
    }
    return enumValue.value;
  }

  parseLiteral(valueNode: ValueNode, _variables: ?ObjMap<mixed>): ?any /* T */ {
    // Note: variables will be resolved to a value before calling this function.
    if (valueNode.kind !== Kind.ENUM) {
      const valueStr = print(valueNode);
      throw new GraphQLError(
        `Enum "${this.name}" cannot represent non-enum value: ${valueStr}.` +
          didYouMeanEnumValue(this, valueStr),
        valueNode,
      );
    }

    const enumValue = this.getValue(valueNode.value);
    if (enumValue == null) {
      const valueStr = print(valueNode);
      throw new GraphQLError(
        `Value "${valueStr}" does not exist in "${this.name}" enum.` +
          didYouMeanEnumValue(this, valueStr),
        valueNode,
      );
    }
    return enumValue.value;
  }

  toConfig(): {|
    ...GraphQLEnumTypeConfig,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<EnumTypeExtensionNode>,
  |} {
    const values = keyValMap(
      this.getValues(),
      (value) => value.name,
      (value) => ({
        description: value.description,
        value: value.value,
        deprecationReason: value.deprecationReason,
        extensions: value.extensions,
        astNode: value.astNode,
      }),
    );

    return {
      name: this.name,
      description: this.description,
      values,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes ?? [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLEnumType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLEnumType);

function didYouMeanEnumValue(
  enumType: GraphQLEnumType,
  unknownValueStr: string,
): string {
  const allNames = enumType.getValues().map((value) => value.name);
  const suggestedValues = suggestionList(unknownValueStr, allNames);

  return didYouMean('the enum value', suggestedValues);
}

function defineEnumValues(
  typeName: string,
  valueMap: GraphQLEnumValueConfigMap /* <T> */,
): Array<GraphQLEnumValue /* <T> */> {
  devAssert(
    isPlainObj(valueMap),
    `${typeName} values must be an object with value names as keys.`,
  );
  return objectEntries(valueMap).map(([valueName, valueConfig]) => {
    devAssert(
      isPlainObj(valueConfig),
      `${typeName}.${valueName} must refer to an object with a "value" key ` +
        `representing an internal value but got: ${inspect(valueConfig)}.`,
    );
    devAssert(
      !('isDeprecated' in valueConfig),
      `${typeName}.${valueName} should provide "deprecationReason" instead of "isDeprecated".`,
    );
    return {
      name: valueName,
      description: valueConfig.description,
      value: valueConfig.value !== undefined ? valueConfig.value : valueName,
      isDeprecated: valueConfig.deprecationReason != null,
      deprecationReason: valueConfig.deprecationReason,
      extensions: valueConfig.extensions && toObjMap(valueConfig.extensions),
      astNode: valueConfig.astNode,
    };
  });
}

export type GraphQLEnumTypeConfig /* <T> */ = {|
  name: string,
  description?: ?string,
  values: GraphQLEnumValueConfigMap /* <T> */,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?EnumTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<EnumTypeExtensionNode>,
|};

export type GraphQLEnumValueConfigMap /* <T> */ = ObjMap<GraphQLEnumValueConfig /* <T> */>;

export type GraphQLEnumValueConfig /* <T> */ = {|
  description?: ?string,
  value?: any /* T */,
  deprecationReason?: ?string,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?EnumValueDefinitionNode,
|};

export type GraphQLEnumValue /* <T> */ = {|
  name: string,
  description: ?string,
  value: any /* T */,
  isDeprecated: boolean,
  deprecationReason: ?string,
  extensions: ?ReadOnlyObjMap<mixed>,
  astNode: ?EnumValueDefinitionNode,
|};

/**
 * Input Object Type Definition
 *
 * An input object defines a structured collection of fields which may be
 * supplied to a field argument.
 *
 * Using `NonNull` will ensure that a value must be provided by the query
 *
 * Example:
 *
 *     const GeoPoint = new GraphQLInputObjectType({
 *       name: 'GeoPoint',
 *       fields: {
 *         lat: { type: GraphQLNonNull(GraphQLFloat) },
 *         lon: { type: GraphQLNonNull(GraphQLFloat) },
 *         alt: { type: GraphQLFloat, defaultValue: 0 },
 *       }
 *     });
 *
 */
export class GraphQLInputObjectType {
  name: string;
  description: ?string;
  extensions: ?ReadOnlyObjMap<mixed>;
  astNode: ?InputObjectTypeDefinitionNode;
  extensionASTNodes: ?$ReadOnlyArray<InputObjectTypeExtensionNode>;

  _fields: Thunk<GraphQLInputFieldMap>;

  constructor(config: $ReadOnly<GraphQLInputObjectTypeConfig>): void {
    this.name = config.name;
    this.description = config.description;
    this.extensions = config.extensions && toObjMap(config.extensions);
    this.astNode = config.astNode;
    this.extensionASTNodes = undefineIfEmpty(config.extensionASTNodes);

    this._fields = defineInputFieldMap.bind(undefined, config);
    devAssert(typeof config.name === 'string', 'Must provide name.');
  }

  getFields(): GraphQLInputFieldMap {
    if (typeof this._fields === 'function') {
      this._fields = this._fields();
    }
    return this._fields;
  }

  toConfig(): {|
    ...GraphQLInputObjectTypeConfig,
    fields: GraphQLInputFieldConfigMap,
    extensions: ?ReadOnlyObjMap<mixed>,
    extensionASTNodes: $ReadOnlyArray<InputObjectTypeExtensionNode>,
  |} {
    const fields = mapValue(this.getFields(), (field) => ({
      description: field.description,
      type: field.type,
      defaultValue: field.defaultValue,
      extensions: field.extensions,
      astNode: field.astNode,
    }));

    return {
      name: this.name,
      description: this.description,
      fields,
      extensions: this.extensions,
      astNode: this.astNode,
      extensionASTNodes: this.extensionASTNodes ?? [],
    };
  }

  toString(): string {
    return this.name;
  }

  toJSON(): string {
    return this.toString();
  }

  // $FlowFixMe Flow doesn't support computed properties yet
  get [SYMBOL_TO_STRING_TAG]() {
    return 'GraphQLInputObjectType';
  }
}

// Print a simplified form when appearing in `inspect` and `util.inspect`.
defineInspect(GraphQLInputObjectType);

function defineInputFieldMap(
  config: $ReadOnly<GraphQLInputObjectTypeConfig>,
): GraphQLInputFieldMap {
  const fieldMap = resolveThunk(config.fields);
  devAssert(
    isPlainObj(fieldMap),
    `${config.name} fields must be an object with field names as keys or a function which returns such an object.`,
  );
  return mapValue(fieldMap, (fieldConfig, fieldName) => {
    devAssert(
      !('resolve' in fieldConfig),
      `${config.name}.${fieldName} field has a resolve property, but Input Types cannot define resolvers.`,
    );

    return {
      name: fieldName,
      description: fieldConfig.description,
      type: fieldConfig.type,
      defaultValue: fieldConfig.defaultValue,
      extensions: fieldConfig.extensions && toObjMap(fieldConfig.extensions),
      astNode: fieldConfig.astNode,
    };
  });
}

export type GraphQLInputObjectTypeConfig = {|
  name: string,
  description?: ?string,
  fields: Thunk<GraphQLInputFieldConfigMap>,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?InputObjectTypeDefinitionNode,
  extensionASTNodes?: ?$ReadOnlyArray<InputObjectTypeExtensionNode>,
|};

export type GraphQLInputFieldConfig = {|
  description?: ?string,
  type: GraphQLInputType,
  defaultValue?: mixed,
  extensions?: ?ReadOnlyObjMapLike<mixed>,
  astNode?: ?InputValueDefinitionNode,
|};

export type GraphQLInputFieldConfigMap = ObjMap<GraphQLInputFieldConfig>;

export type GraphQLInputField = {|
  name: string,
  description: ?string,
  type: GraphQLInputType,
  defaultValue: mixed,
  extensions: ?ReadOnlyObjMap<mixed>,
  astNode: ?InputValueDefinitionNode,
|};

export function isRequiredInputField(
  field: GraphQLInputField,
): boolean %checks {
  return isNonNullType(field.type) && field.defaultValue === undefined;
}

export type GraphQLInputFieldMap = ObjMap<GraphQLInputField>;
