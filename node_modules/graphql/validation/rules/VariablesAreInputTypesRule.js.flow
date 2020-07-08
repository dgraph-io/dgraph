// @flow strict

import { GraphQLError } from '../../error/GraphQLError';

import { print } from '../../language/printer';
import type { ASTVisitor } from '../../language/visitor';
import type { VariableDefinitionNode } from '../../language/ast';

import { isInputType } from '../../type/definition';

import { typeFromAST } from '../../utilities/typeFromAST';

import type { ValidationContext } from '../ValidationContext';

/**
 * Variables are input types
 *
 * A GraphQL operation is only valid if all the variables it defines are of
 * input types (scalar, enum, or input object).
 */
export function VariablesAreInputTypesRule(
  context: ValidationContext,
): ASTVisitor {
  return {
    VariableDefinition(node: VariableDefinitionNode): ?GraphQLError {
      const type = typeFromAST(context.getSchema(), node.type);

      if (type && !isInputType(type)) {
        const variableName = node.variable.name.value;
        const typeName = print(node.type);

        context.reportError(
          new GraphQLError(
            `Variable "$${variableName}" cannot be non-input type "${typeName}".`,
            node.type,
          ),
        );
      }
    },
  };
}
