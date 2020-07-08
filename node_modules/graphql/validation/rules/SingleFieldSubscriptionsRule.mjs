import { GraphQLError } from "../../error/GraphQLError.mjs";

/**
 * Subscriptions must only include one field.
 *
 * A GraphQL subscription is valid only if it contains a single root field.
 */
export function SingleFieldSubscriptionsRule(context) {
  return {
    OperationDefinition: function OperationDefinition(node) {
      if (node.operation === 'subscription') {
        if (node.selectionSet.selections.length !== 1) {
          context.reportError(new GraphQLError(node.name ? "Subscription \"".concat(node.name.value, "\" must select only one top level field.") : 'Anonymous Subscription must select only one top level field.', node.selectionSet.selections.slice(1)));
        }
      }
    }
  };
}
