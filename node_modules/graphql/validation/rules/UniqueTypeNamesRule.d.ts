import { ASTVisitor } from '../../language/visitor';
import { SDLValidationContext } from '../ValidationContext';

/**
 * Unique type names
 *
 * A GraphQL document is only valid if all defined types have unique names.
 */
export function UniqueTypeNamesRule(context: SDLValidationContext): ASTVisitor;
