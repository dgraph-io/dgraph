// @flow strict

import type { GraphQLError } from '../error/GraphQLError';

import type { DocumentNode } from '../language/ast';

import type { GraphQLSchema } from '../type/schema';

import { validate } from '../validation/validate';
import { NoDeprecatedCustomRule } from '../validation/rules/custom/NoDeprecatedCustomRule';

/**
 * A validation rule which reports deprecated usages.
 *
 * Returns a list of GraphQLError instances describing each deprecated use.
 *
 * @deprecated Please use `validate` with `NoDeprecatedCustomRule` instead:
 *
 * ```
 * import { validate, NoDeprecatedCustomRule } from 'graphql'
 *
 * const errors = validate(schema, document, [NoDeprecatedCustomRule])
 * ```
 */
export function findDeprecatedUsages(
  schema: GraphQLSchema,
  ast: DocumentNode,
): $ReadOnlyArray<GraphQLError> {
  return validate(schema, ast, [NoDeprecatedCustomRule]);
}
