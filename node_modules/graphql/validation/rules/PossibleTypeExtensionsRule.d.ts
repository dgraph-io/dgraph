import { ASTVisitor } from '../../language/visitor';
import { SDLValidationContext } from '../ValidationContext';

/**
 * Possible type extension
 *
 * A type extension is only valid if the type is defined and has the same kind.
 */
export function PossibleTypeExtensionsRule(
  context: SDLValidationContext,
): ASTVisitor;
