// FIXME
/* eslint-disable import/no-cycle */

import { Maybe } from '../jsutils/Maybe';

import { DirectiveDefinitionNode } from '../language/ast';
import { DirectiveLocationEnum } from '../language/directiveLocation';

import { GraphQLFieldConfigArgumentMap, GraphQLArgument } from './definition';

/**
 * Test if the given value is a GraphQL directive.
 */
export function isDirective(directive: any): directive is GraphQLDirective;
export function assertDirective(directive: any): GraphQLDirective;

/**
 * Custom extensions
 *
 * @remarks
 * Use a unique identifier name for your extension, for example the name of
 * your library or project. Do not use a shortened identifier as this increases
 * the risk of conflicts. We recommend you add at most one extension field,
 * an object which can contain all the values you need.
 */
export interface GraphQLDirectiveExtensions {
  [attributeName: string]: any;
}

/**
 * Directives are used by the GraphQL runtime as a way of modifying execution
 * behavior. Type system creators will usually not create these directly.
 */
export class GraphQLDirective {
  name: string;
  description: Maybe<string>;
  locations: Array<DirectiveLocationEnum>;
  isRepeatable: boolean;
  args: Array<GraphQLArgument>;
  extensions: Maybe<Readonly<GraphQLDirectiveExtensions>>;
  astNode: Maybe<DirectiveDefinitionNode>;

  constructor(config: Readonly<GraphQLDirectiveConfig>);

  toConfig(): GraphQLDirectiveConfig & {
    args: GraphQLFieldConfigArgumentMap;
    isRepeatable: boolean;
    extensions: Maybe<Readonly<GraphQLDirectiveExtensions>>;
  };

  toString(): string;
  toJSON(): string;
  inspect(): string;
}

export interface GraphQLDirectiveConfig {
  name: string;
  description?: Maybe<string>;
  locations: Array<DirectiveLocationEnum>;
  args?: Maybe<GraphQLFieldConfigArgumentMap>;
  isRepeatable?: Maybe<boolean>;
  extensions?: Maybe<Readonly<GraphQLDirectiveExtensions>>;
  astNode?: Maybe<DirectiveDefinitionNode>;
}

/**
 * Used to conditionally include fields or fragments.
 */
export const GraphQLIncludeDirective: GraphQLDirective;

/**
 * Used to conditionally skip (exclude) fields or fragments.
 */
export const GraphQLSkipDirective: GraphQLDirective;

/**
 * Used to provide a URL for specifying the behavior of custom scalar definitions.
 */
export const GraphQLSpecifiedByDirective: GraphQLDirective;

/**
 * Constant string used for default reason for a deprecation.
 */
export const DEFAULT_DEPRECATION_REASON: 'No longer supported';

/**
 * Used to declare element of a GraphQL schema as deprecated.
 */
export const GraphQLDeprecatedDirective: GraphQLDirective;

/**
 * The full list of specified directives.
 */
export const specifiedDirectives: ReadonlyArray<GraphQLDirective>;

export function isSpecifiedDirective(directive: GraphQLDirective): boolean;
