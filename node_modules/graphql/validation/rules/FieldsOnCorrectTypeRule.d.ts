import { ASTVisitor } from '../../language/visitor';
import { ValidationContext } from '../ValidationContext';

/**
 * Fields on correct type
 *
 * A GraphQL document is only valid if all fields selected are defined by the
 * parent type, or are an allowed meta field such as __typename.
 */
export function FieldsOnCorrectTypeRule(context: ValidationContext): ASTVisitor;
