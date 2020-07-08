// @flow strict

import invariant from '../jsutils/invariant';

import { parse } from '../language/parser';

import type { GraphQLSchema } from '../type/schema';

import { executeSync } from '../execution/execute';

import type {
  IntrospectionQuery,
  IntrospectionOptions,
} from './getIntrospectionQuery';
import { getIntrospectionQuery } from './getIntrospectionQuery';

/**
 * Build an IntrospectionQuery from a GraphQLSchema
 *
 * IntrospectionQuery is useful for utilities that care about type and field
 * relationships, but do not need to traverse through those relationships.
 *
 * This is the inverse of buildClientSchema. The primary use case is outside
 * of the server context, for instance when doing schema comparisons.
 */
export function introspectionFromSchema(
  schema: GraphQLSchema,
  options?: IntrospectionOptions,
): IntrospectionQuery {
  const optionsWithDefaults = {
    directiveIsRepeatable: true,
    schemaDescription: true,
    ...options,
  };

  const document = parse(getIntrospectionQuery(optionsWithDefaults));
  const result = executeSync({ schema, document });
  invariant(!result.errors && result.data);
  return (result.data: any);
}
