// @flow strict

import { GraphQLError } from '../../error/GraphQLError';

import { Kind } from '../../language/kinds';
import type { ASTVisitor } from '../../language/visitor';
import {
  isTypeDefinitionNode,
  isTypeExtensionNode,
} from '../../language/predicates';

import { specifiedDirectives } from '../../type/directives';

import type {
  SDLValidationContext,
  ValidationContext,
} from '../ValidationContext';

/**
 * Unique directive names per location
 *
 * A GraphQL document is only valid if all non-repeatable directives at
 * a given location are uniquely named.
 */
export function UniqueDirectivesPerLocationRule(
  context: ValidationContext | SDLValidationContext,
): ASTVisitor {
  const uniqueDirectiveMap = Object.create(null);

  const schema = context.getSchema();
  const definedDirectives = schema
    ? schema.getDirectives()
    : specifiedDirectives;
  for (const directive of definedDirectives) {
    uniqueDirectiveMap[directive.name] = !directive.isRepeatable;
  }

  const astDefinitions = context.getDocument().definitions;
  for (const def of astDefinitions) {
    if (def.kind === Kind.DIRECTIVE_DEFINITION) {
      uniqueDirectiveMap[def.name.value] = !def.repeatable;
    }
  }

  const schemaDirectives = Object.create(null);
  const typeDirectivesMap = Object.create(null);

  return {
    // Many different AST nodes may contain directives. Rather than listing
    // them all, just listen for entering any node, and check to see if it
    // defines any directives.
    enter(node) {
      if (node.directives == null) {
        return;
      }

      let seenDirectives;
      if (
        node.kind === Kind.SCHEMA_DEFINITION ||
        node.kind === Kind.SCHEMA_EXTENSION
      ) {
        seenDirectives = schemaDirectives;
      } else if (isTypeDefinitionNode(node) || isTypeExtensionNode(node)) {
        const typeName = node.name.value;
        seenDirectives = typeDirectivesMap[typeName];
        if (seenDirectives === undefined) {
          typeDirectivesMap[typeName] = seenDirectives = Object.create(null);
        }
      } else {
        seenDirectives = Object.create(null);
      }

      for (const directive of node.directives) {
        const directiveName = directive.name.value;

        if (uniqueDirectiveMap[directiveName]) {
          if (seenDirectives[directiveName]) {
            context.reportError(
              new GraphQLError(
                `The directive "@${directiveName}" can only be used once at this location.`,
                [seenDirectives[directiveName], directive],
              ),
            );
          } else {
            seenDirectives[directiveName] = directive;
          }
        }
      }
    },
  };
}
