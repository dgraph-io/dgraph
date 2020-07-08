"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.findDeprecatedUsages = findDeprecatedUsages;

var _validate = require("../validation/validate");

var _NoDeprecatedCustomRule = require("../validation/rules/custom/NoDeprecatedCustomRule");

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
function findDeprecatedUsages(schema, ast) {
  return (0, _validate.validate)(schema, ast, [_NoDeprecatedCustomRule.NoDeprecatedCustomRule]);
}
