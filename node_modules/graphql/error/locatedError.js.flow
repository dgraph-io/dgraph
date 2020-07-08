// @flow strict

import type { ASTNode } from '../language/ast';

import { GraphQLError } from './GraphQLError';

/**
 * Given an arbitrary Error, presumably thrown while attempting to execute a
 * GraphQL operation, produce a new GraphQLError aware of the location in the
 * document responsible for the original Error.
 */
export function locatedError(
  originalError: Error | GraphQLError,
  nodes: ASTNode | $ReadOnlyArray<ASTNode> | void | null,
  path?: ?$ReadOnlyArray<string | number>,
): GraphQLError {
  // Note: this uses a brand-check to support GraphQL errors originating from
  // other contexts.
  if (Array.isArray(originalError.path)) {
    return (originalError: any);
  }

  return new GraphQLError(
    originalError.message,
    (originalError: any).nodes ?? nodes,
    (originalError: any).source,
    (originalError: any).positions,
    path,
    originalError,
  );
}
