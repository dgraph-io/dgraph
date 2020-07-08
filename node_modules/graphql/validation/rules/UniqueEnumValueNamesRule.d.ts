import { ASTVisitor } from '../../language/visitor';
import { SDLValidationContext } from '../ValidationContext';

/**
 * Unique enum value names
 *
 * A GraphQL enum type is only valid if all its values are uniquely named.
 */
export function UniqueEnumValueNamesRule(
  context: SDLValidationContext,
): ASTVisitor;
