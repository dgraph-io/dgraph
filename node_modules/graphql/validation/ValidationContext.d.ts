import { Maybe } from '../jsutils/Maybe';

import { GraphQLError } from '../error/GraphQLError';
import { ASTVisitor } from '../language/visitor';
import {
  DocumentNode,
  OperationDefinitionNode,
  VariableNode,
  SelectionSetNode,
  FragmentSpreadNode,
  FragmentDefinitionNode,
} from '../language/ast';
import { GraphQLSchema } from '../type/schema';
import { GraphQLDirective } from '../type/directives';
import {
  GraphQLInputType,
  GraphQLOutputType,
  GraphQLCompositeType,
  GraphQLField,
  GraphQLArgument,
  GraphQLEnumValue,
} from '../type/definition';
import { TypeInfo } from '../utilities/TypeInfo';

type NodeWithSelectionSet = OperationDefinitionNode | FragmentDefinitionNode;
interface VariableUsage {
  readonly node: VariableNode;
  readonly type: Maybe<GraphQLInputType>;
  readonly defaultValue: Maybe<any>;
}

/**
 * An instance of this class is passed as the "this" context to all validators,
 * allowing access to commonly useful contextual information from within a
 * validation rule.
 */
export class ASTValidationContext {
  constructor(ast: DocumentNode, onError: (err: GraphQLError) => void);

  reportError(error: GraphQLError): undefined;

  getDocument(): DocumentNode;

  getFragment(name: string): Maybe<FragmentDefinitionNode>;

  getFragmentSpreads(node: SelectionSetNode): ReadonlyArray<FragmentSpreadNode>;

  getRecursivelyReferencedFragments(
    operation: OperationDefinitionNode,
  ): ReadonlyArray<FragmentDefinitionNode>;
}

export class SDLValidationContext extends ASTValidationContext {
  constructor(
    ast: DocumentNode,
    schema: Maybe<GraphQLSchema>,
    onError: (err: GraphQLError) => void,
  );

  getSchema(): Maybe<GraphQLSchema>;
}

export type SDLValidationRule = (context: SDLValidationContext) => ASTVisitor;

export class ValidationContext extends ASTValidationContext {
  constructor(
    schema: GraphQLSchema,
    ast: DocumentNode,
    typeInfo: TypeInfo,
    onError: (err: GraphQLError) => void,
  );

  getSchema(): GraphQLSchema;

  getVariableUsages(node: NodeWithSelectionSet): ReadonlyArray<VariableUsage>;

  getRecursivelyReferencedFragments(
    operation: OperationDefinitionNode,
  ): ReadonlyArray<FragmentDefinitionNode>;

  getType(): Maybe<GraphQLOutputType>;

  getParentType(): Maybe<GraphQLCompositeType>;

  getInputType(): Maybe<GraphQLInputType>;

  getParentInputType(): Maybe<GraphQLInputType>;

  getFieldDef(): Maybe<GraphQLField<any, any>>;

  getDirective(): Maybe<GraphQLDirective>;

  getArgument(): Maybe<GraphQLArgument>;

  getEnumValue(): Maybe<GraphQLEnumValue>;
}

export type ValidationRule = (context: ValidationContext) => ASTVisitor;
