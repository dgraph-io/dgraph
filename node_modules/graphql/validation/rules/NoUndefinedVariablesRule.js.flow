// @flow strict

import { GraphQLError } from '../../error/GraphQLError';

import type { ASTVisitor } from '../../language/visitor';

import type { ValidationContext } from '../ValidationContext';

/**
 * No undefined variables
 *
 * A GraphQL operation is only valid if all variables encountered, both directly
 * and via fragment spreads, are defined by that operation.
 */
export function NoUndefinedVariablesRule(
  context: ValidationContext,
): ASTVisitor {
  let variableNameDefined = Object.create(null);

  return {
    OperationDefinition: {
      enter() {
        variableNameDefined = Object.create(null);
      },
      leave(operation) {
        const usages = context.getRecursiveVariableUsages(operation);

        for (const { node } of usages) {
          const varName = node.name.value;
          if (variableNameDefined[varName] !== true) {
            context.reportError(
              new GraphQLError(
                operation.name
                  ? `Variable "$${varName}" is not defined by operation "${operation.name.value}".`
                  : `Variable "$${varName}" is not defined.`,
                [node, operation],
              ),
            );
          }
        }
      },
    },
    VariableDefinition(node) {
      variableNameDefined[node.variable.name.value] = true;
    },
  };
}
