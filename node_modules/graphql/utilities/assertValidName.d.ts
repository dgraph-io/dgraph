import { GraphQLError } from '../error/GraphQLError';

/**
 * Upholds the spec rules about naming.
 */
export function assertValidName(name: string): string;

/**
 * Returns an Error if a name is invalid.
 */
export function isValidNameError(name: string): GraphQLError | undefined;
