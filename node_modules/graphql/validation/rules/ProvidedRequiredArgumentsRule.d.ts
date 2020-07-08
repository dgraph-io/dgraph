import { ASTVisitor } from '../../language/visitor';
import { ValidationContext, SDLValidationContext } from '../ValidationContext';

/**
 * Provided required arguments
 *
 * A field or directive is only valid if all required (non-null without a
 * default value) field arguments have been provided.
 */
export function ProvidedRequiredArgumentsRule(
  context: ValidationContext,
): ASTVisitor;

/**
 * @internal
 */
export function ProvidedRequiredArgumentsOnDirectivesRule(
  context: ValidationContext | SDLValidationContext,
): ASTVisitor;
