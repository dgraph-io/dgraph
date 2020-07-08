// @flow strict

import devAssert from '../jsutils/devAssert';

import type { SourceLocation } from '../language/location';

import type { GraphQLError } from './GraphQLError';

/**
 * Given a GraphQLError, format it according to the rules described by the
 * Response Format, Errors section of the GraphQL Specification.
 */
export function formatError(error: GraphQLError): GraphQLFormattedError {
  devAssert(error, 'Received null or undefined error.');
  const message = error.message ?? 'An unknown error occurred.';
  const locations = error.locations;
  const path = error.path;
  const extensions = error.extensions;

  return extensions
    ? { message, locations, path, extensions }
    : { message, locations, path };
}

/**
 * @see https://github.com/graphql/graphql-spec/blob/master/spec/Section%207%20--%20Response.md#errors
 */
export type GraphQLFormattedError = {|
  /**
   * A short, human-readable summary of the problem that **SHOULD NOT** change
   * from occurrence to occurrence of the problem, except for purposes of
   * localization.
   */
  +message: string,
  /**
   * If an error can be associated to a particular point in the requested
   * GraphQL document, it should contain a list of locations.
   */
  +locations: $ReadOnlyArray<SourceLocation> | void,
  /**
   * If an error can be associated to a particular field in the GraphQL result,
   * it _must_ contain an entry with the key `path` that details the path of
   * the response field which experienced the error. This allows clients to
   * identify whether a null result is intentional or caused by a runtime error.
   */
  +path: $ReadOnlyArray<string | number> | void,
  /**
   * Reserved for implementors to extend the protocol however they see fit,
   * and hence there are no additional restrictions on its contents.
   */
  +extensions?: { [key: string]: mixed, ... },
|};
