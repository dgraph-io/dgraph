import { Maybe } from '../jsutils/Maybe';

import { ASTNode } from '../language/ast';

import { GraphQLError } from './GraphQLError';

/**
 * Given an arbitrary Error, presumably thrown while attempting to execute a
 * GraphQL operation, produce a new GraphQLError aware of the location in the
 * document responsible for the original Error.
 */
export function locatedError(
  originalError: Error | GraphQLError,
  nodes: ASTNode | ReadonlyArray<ASTNode> | undefined,
  path?: Maybe<ReadonlyArray<string | number>>,
): GraphQLError;
