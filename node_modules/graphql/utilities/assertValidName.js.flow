// @flow strict

import devAssert from '../jsutils/devAssert';

import { GraphQLError } from '../error/GraphQLError';

const NAME_RX = /^[_a-zA-Z][_a-zA-Z0-9]*$/;

/**
 * Upholds the spec rules about naming.
 */
export function assertValidName(name: string): string {
  const error = isValidNameError(name);
  if (error) {
    throw error;
  }
  return name;
}

/**
 * Returns an Error if a name is invalid.
 */
export function isValidNameError(name: string): GraphQLError | void {
  devAssert(typeof name === 'string', 'Expected name to be a string.');
  if (name.length > 1 && name[0] === '_' && name[1] === '_') {
    return new GraphQLError(
      `Name "${name}" must not begin with "__", which is reserved by GraphQL introspection.`,
    );
  }
  if (!NAME_RX.test(name)) {
    return new GraphQLError(
      `Names must match /^[_a-zA-Z][_a-zA-Z0-9]*$/ but "${name}" does not.`,
    );
  }
}
