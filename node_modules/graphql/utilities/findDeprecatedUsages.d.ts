import { GraphQLError } from '../error/GraphQLError';
import { DocumentNode } from '../language/ast';
import { GraphQLSchema } from '../type/schema';

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
): ReadonlyArray<GraphQLError>;
