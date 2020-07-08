// @flow strict

import { GraphQLError } from '../../error/GraphQLError';

import type { ASTVisitor } from '../../language/visitor';
import { Kind } from '../../language/kinds';
import { isExecutableDefinitionNode } from '../../language/predicates';

import type { ASTValidationContext } from '../ValidationContext';

/**
 * Executable definitions
 *
 * A GraphQL document is only valid for execution if all definitions are either
 * operation or fragment definitions.
 */
export function ExecutableDefinitionsRule(
  context: ASTValidationContext,
): ASTVisitor {
  return {
    Document(node) {
      for (const definition of node.definitions) {
        if (!isExecutableDefinitionNode(definition)) {
          const defName =
            definition.kind === Kind.SCHEMA_DEFINITION ||
            definition.kind === Kind.SCHEMA_EXTENSION
              ? 'schema'
              : '"' + definition.name.value + '"';
          context.reportError(
            new GraphQLError(
              `The ${defName} definition is not executable.`,
              definition,
            ),
          );
        }
      }
      return false;
    },
  };
}
