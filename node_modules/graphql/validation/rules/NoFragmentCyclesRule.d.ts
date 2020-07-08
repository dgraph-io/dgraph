import { ASTVisitor } from '../../language/visitor';
import { ValidationContext } from '../ValidationContext';

export function NoFragmentCyclesRule(context: ValidationContext): ASTVisitor;
