// @flow strict

import { GraphQLError } from '../../error/GraphQLError';

import type { ASTVisitor } from '../../language/visitor';
import type { OperationDefinitionNode } from '../../language/ast';

import type { ASTValidationContext } from '../ValidationContext';

/**
 * Subscriptions must only include one field.
 *
 * A GraphQL subscription is valid only if it contains a single root field.
 */
export function SingleFieldSubscriptionsRule(
  context: ASTValidationContext,
): ASTVisitor {
  return {
    OperationDefinition(node: OperationDefinitionNode) {
      if (node.operation === 'subscription') {
        if (node.selectionSet.selections.length !== 1) {
          context.reportError(
            new GraphQLError(
              node.name
                ? `Subscription "${node.name.value}" must select only one top level field.`
                : 'Anonymous Subscription must select only one top level field.',
              node.selectionSet.selections.slice(1),
            ),
          );
        }
      }
    },
  };
}
