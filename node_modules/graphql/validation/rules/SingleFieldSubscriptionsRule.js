"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.SingleFieldSubscriptionsRule = SingleFieldSubscriptionsRule;

var _GraphQLError = require("../../error/GraphQLError");

/**
 * Subscriptions must only include one field.
 *
 * A GraphQL subscription is valid only if it contains a single root field.
 */
function SingleFieldSubscriptionsRule(context) {
  return {
    OperationDefinition: function OperationDefinition(node) {
      if (node.operation === 'subscription') {
        if (node.selectionSet.selections.length !== 1) {
          context.reportError(new _GraphQLError.GraphQLError(node.name ? "Subscription \"".concat(node.name.value, "\" must select only one top level field.") : 'Anonymous Subscription must select only one top level field.', node.selectionSet.selections.slice(1)));
        }
      }
    }
  };
}
