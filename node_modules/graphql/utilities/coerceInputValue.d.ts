import { GraphQLInputType } from '../type/definition';
import { GraphQLError } from '../error/GraphQLError';

type OnErrorCB = (
  path: ReadonlyArray<string | number>,
  invalidValue: any,
  error: GraphQLError,
) => void;

/**
 * Coerces a JavaScript value given a GraphQL Input Type.
 */
export function coerceInputValue(
  inputValue: any,
  type: GraphQLInputType,
  onError?: OnErrorCB,
): any;
