// @flow strict

import inspect from '../jsutils/inspect';
import invariant from '../jsutils/invariant';

import type {
  NamedTypeNode,
  ListTypeNode,
  NonNullTypeNode,
} from '../language/ast';

import { Kind } from '../language/kinds';

import type { GraphQLSchema } from '../type/schema';
import type { GraphQLNamedType } from '../type/definition';
import { GraphQLList, GraphQLNonNull } from '../type/definition';

/**
 * Given a Schema and an AST node describing a type, return a GraphQLType
 * definition which applies to that type. For example, if provided the parsed
 * AST node for `[User]`, a GraphQLList instance will be returned, containing
 * the type called "User" found in the schema. If a type called "User" is not
 * found in the schema, then undefined will be returned.
 */
/* eslint-disable no-redeclare */
declare function typeFromAST(
  schema: GraphQLSchema,
  typeNode: NamedTypeNode,
): GraphQLNamedType | void;
declare function typeFromAST(
  schema: GraphQLSchema,
  typeNode: ListTypeNode,
): GraphQLList<any> | void;
declare function typeFromAST(
  schema: GraphQLSchema,
  typeNode: NonNullTypeNode,
): GraphQLNonNull<any> | void;
export function typeFromAST(schema, typeNode) {
  /* eslint-enable no-redeclare */
  let innerType;
  if (typeNode.kind === Kind.LIST_TYPE) {
    innerType = typeFromAST(schema, typeNode.type);
    return innerType && GraphQLList(innerType);
  }
  if (typeNode.kind === Kind.NON_NULL_TYPE) {
    innerType = typeFromAST(schema, typeNode.type);
    return innerType && GraphQLNonNull(innerType);
  }
  // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')
  if (typeNode.kind === Kind.NAMED_TYPE) {
    return schema.getType(typeNode.name.value);
  }

  // istanbul ignore next (Not reachable. All possible type nodes have been considered)
  invariant(false, 'Unexpected type node: ' + inspect((typeNode: empty)));
}
