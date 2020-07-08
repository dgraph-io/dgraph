import { ASTVisitor } from '../../language/visitor';
import { ASTValidationContext } from '../ValidationContext';

/**
 * Unique operation names
 *
 * A GraphQL document is only valid if all defined operations have unique names.
 */
export function UniqueOperationNamesRule(
  context: ASTValidationContext,
): ASTVisitor;
