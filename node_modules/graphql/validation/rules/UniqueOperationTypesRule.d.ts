import { ASTVisitor } from '../../language/visitor';
import { SDLValidationContext } from '../ValidationContext';

/**
 * Unique operation types
 *
 * A GraphQL document is only valid if it has only one type per operation.
 */
export function UniqueOperationTypesRule(
  context: SDLValidationContext,
): ASTVisitor;
