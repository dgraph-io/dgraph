import { Maybe } from '../jsutils/Maybe';

import { Visitor } from '../language/visitor';
import { ASTNode, ASTKindToNode, FieldNode } from '../language/ast';
import { GraphQLSchema } from '../type/schema';
import { GraphQLDirective } from '../type/directives';
import {
  GraphQLType,
  GraphQLInputType,
  GraphQLOutputType,
  GraphQLCompositeType,
  GraphQLField,
  GraphQLArgument,
  GraphQLEnumValue,
} from '../type/definition';

/**
 * TypeInfo is a utility class which, given a GraphQL schema, can keep track
 * of the current field and type definitions at any point in a GraphQL document
 * AST during a recursive descent by calling `enter(node)` and `leave(node)`.
 */
export class TypeInfo {
  constructor(
    schema: GraphQLSchema,
    // NOTE: this experimental optional second parameter is only needed in order
    // to support non-spec-compliant code bases. You should never need to use it.
    // It may disappear in the future.
    getFieldDefFn?: getFieldDef,
    // Initial type may be provided in rare cases to facilitate traversals
    // beginning somewhere other than documents.
    initialType?: GraphQLType,
  );

  getType(): Maybe<GraphQLOutputType>;
  getParentType(): Maybe<GraphQLCompositeType>;
  getInputType(): Maybe<GraphQLInputType>;
  getParentInputType(): Maybe<GraphQLInputType>;
  getFieldDef(): GraphQLField<any, Maybe<any>>;
  getDefaultValue(): Maybe<any>;
  getDirective(): Maybe<GraphQLDirective>;
  getArgument(): Maybe<GraphQLArgument>;
  getEnumValue(): Maybe<GraphQLEnumValue>;
  enter(node: ASTNode): any;
  leave(node: ASTNode): any;
}

type getFieldDef = (
  schema: GraphQLSchema,
  parentType: GraphQLType,
  fieldNode: FieldNode,
) => Maybe<GraphQLField<any, any>>;

/**
 * Creates a new visitor instance which maintains a provided TypeInfo instance
 * along with visiting visitor.
 */
export function visitWithTypeInfo(
  typeInfo: TypeInfo,
  visitor: Visitor<ASTKindToNode>,
): Visitor<ASTKindToNode>;
