import { ASTVisitor } from '../../language/visitor';
import { ValidationContext } from '../ValidationContext';

/**
 * Scalar leafs
 *
 * A GraphQL document is valid only if all leaf fields (fields without
 * sub selections) are of scalar or enum types.
 */
export function ScalarLeafsRule(context: ValidationContext): ASTVisitor;
