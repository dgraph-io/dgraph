import { ASTVisitor } from '../../language/visitor';
import { ValidationContext, SDLValidationContext } from '../ValidationContext';

/**
 * Known type names
 *
 * A GraphQL document is only valid if referenced types (specifically
 * variable definitions and fragment conditions) are defined by the type schema.
 */
export function KnownTypeNamesRule(
  context: ValidationContext | SDLValidationContext,
): ASTVisitor;
