import { ASTVisitor } from '../../language/visitor';
import { ASTValidationContext } from '../ValidationContext';

/**
 * Subscriptions must only include one field.
 *
 * A GraphQL subscription is valid only if it contains a single root field.
 */
export function SingleFieldSubscriptionsRule(
  context: ASTValidationContext,
): ASTVisitor;
