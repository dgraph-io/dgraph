import { ASTVisitor } from '../../language/visitor';
import { SDLValidationContext } from '../ValidationContext';

/**
 * Unique directive names
 *
 * A GraphQL document is only valid if all defined directives have unique names.
 */
export function UniqueDirectiveNamesRule(
  context: SDLValidationContext,
): ASTVisitor;
