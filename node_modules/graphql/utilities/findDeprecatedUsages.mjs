import { validate } from "../validation/validate.mjs";
import { NoDeprecatedCustomRule } from "../validation/rules/custom/NoDeprecatedCustomRule.mjs";
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

export function findDeprecatedUsages(schema, ast) {
  return validate(schema, ast, [NoDeprecatedCustomRule]);
}
