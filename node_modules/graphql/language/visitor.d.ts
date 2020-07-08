import { Maybe } from '../jsutils/Maybe';

import { ASTNode, ASTKindToNode } from './ast';

/**
 * A visitor is provided to visit, it contains the collection of
 * relevant functions to be called during the visitor's traversal.
 */
export type ASTVisitor = Visitor<ASTKindToNode>;
export type Visitor<KindToNode, Nodes = KindToNode[keyof KindToNode]> =
  | EnterLeaveVisitor<KindToNode, Nodes>
  | ShapeMapVisitor<KindToNode, Nodes>;

interface EnterLeave<T> {
  readonly enter?: T;
  readonly leave?: T;
}

type EnterLeaveVisitor<KindToNode, Nodes> = EnterLeave<
  VisitFn<Nodes> | { [K in keyof KindToNode]?: VisitFn<Nodes, KindToNode[K]> }
>;

type ShapeMapVisitor<KindToNode, Nodes> = {
  [K in keyof KindToNode]?:
    | VisitFn<Nodes, KindToNode[K]>
    | EnterLeave<VisitFn<Nodes, KindToNode[K]>>;
};

/**
 * A visitor is comprised of visit functions, which are called on each node
 * during the visitor's traversal.
 */
export type VisitFn<TAnyNode, TVisitedNode = TAnyNode> = (
  /** The current node being visiting. */
  node: TVisitedNode,
  /** The index or key to this node from the parent node or Array. */
  key: string | number | undefined,
  /** The parent immediately above this node, which may be an Array. */
  parent: TAnyNode | ReadonlyArray<TAnyNode> | undefined,
  /** The key path to get to this node from the root node. */
  path: ReadonlyArray<string | number>,
  /**
   * All nodes and Arrays visited before reaching parent of this node.
   * These correspond to array indices in `path`.
   * Note: ancestors includes arrays which contain the parent of visited node.
   */
  ancestors: ReadonlyArray<TAnyNode | ReadonlyArray<TAnyNode>>,
) => any;

/**
 * A KeyMap describes each the traversable properties of each kind of node.
 */
export type VisitorKeyMap<T> = { [P in keyof T]: ReadonlyArray<keyof T[P]> };

// TODO: Should be `[]`, but that requires TypeScript@3
type EmptyTuple = Array<never>;

export const QueryDocumentKeys: {
  Name: EmptyTuple;

  Document: ['definitions'];
  // Prettier forces trailing commas, but TS pre 3.2 doesn't allow them.
  // prettier-ignore
  OperationDefinition: [
    'name',
    'variableDefinitions',
    'directives',
    'selectionSet'
  ];
  VariableDefinition: ['variable', 'type', 'defaultValue', 'directives'];
  Variable: ['name'];
  SelectionSet: ['selections'];
  Field: ['alias', 'name', 'arguments', 'directives', 'selectionSet'];
  Argument: ['name', 'value'];

  FragmentSpread: ['name', 'directives'];
  InlineFragment: ['typeCondition', 'directives', 'selectionSet'];
  // prettier-ignore
  FragmentDefinition: [
    'name',
    // Note: fragment variable definitions are experimental and may be changed
    // or removed in the future.
    'variableDefinitions',
    'typeCondition',
    'directives',
    'selectionSet'
  ];

  IntValue: EmptyTuple;
  FloatValue: EmptyTuple;
  StringValue: EmptyTuple;
  BooleanValue: EmptyTuple;
  NullValue: EmptyTuple;
  EnumValue: EmptyTuple;
  ListValue: ['values'];
  ObjectValue: ['fields'];
  ObjectField: ['name', 'value'];

  Directive: ['name', 'arguments'];

  NamedType: ['name'];
  ListType: ['type'];
  NonNullType: ['type'];

  SchemaDefinition: ['description', 'directives', 'operationTypes'];
  OperationTypeDefinition: ['type'];

  ScalarTypeDefinition: ['description', 'name', 'directives'];
  // prettier-ignore
  ObjectTypeDefinition: [
    'description',
    'name',
    'interfaces',
    'directives',
    'fields'
  ];
  FieldDefinition: ['description', 'name', 'arguments', 'type', 'directives'];
  // prettier-ignore
  InputValueDefinition: [
    'description',
    'name',
    'type',
    'defaultValue',
    'directives'
  ];
  // prettier-ignore
  InterfaceTypeDefinition: [
    'description',
    'name',
    'interfaces',
    'directives',
    'fields'
  ];
  UnionTypeDefinition: ['description', 'name', 'directives', 'types'];
  EnumTypeDefinition: ['description', 'name', 'directives', 'values'];
  EnumValueDefinition: ['description', 'name', 'directives'];
  InputObjectTypeDefinition: ['description', 'name', 'directives', 'fields'];

  DirectiveDefinition: ['description', 'name', 'arguments', 'locations'];

  SchemaExtension: ['directives', 'operationTypes'];

  ScalarTypeExtension: ['name', 'directives'];
  ObjectTypeExtension: ['name', 'interfaces', 'directives', 'fields'];
  InterfaceTypeExtension: ['name', 'interfaces', 'directives', 'fields'];
  UnionTypeExtension: ['name', 'directives', 'types'];
  EnumTypeExtension: ['name', 'directives', 'values'];
  InputObjectTypeExtension: ['name', 'directives', 'fields'];
};

export const BREAK: any;

/**
 * visit() will walk through an AST using a depth-first traversal, calling
 * the visitor's enter function at each node in the traversal, and calling the
 * leave function after visiting that node and all of its child nodes.
 *
 * By returning different values from the enter and leave functions, the
 * behavior of the visitor can be altered, including skipping over a sub-tree of
 * the AST (by returning false), editing the AST by returning a value or null
 * to remove the value, or to stop the whole traversal by returning BREAK.
 *
 * When using visit() to edit an AST, the original AST will not be modified, and
 * a new version of the AST with the changes applied will be returned from the
 * visit function.
 *
 *     const editedAST = visit(ast, {
 *       enter(node, key, parent, path, ancestors) {
 *         // @return
 *         //   undefined: no action
 *         //   false: skip visiting this node
 *         //   visitor.BREAK: stop visiting altogether
 *         //   null: delete this node
 *         //   any value: replace this node with the returned value
 *       },
 *       leave(node, key, parent, path, ancestors) {
 *         // @return
 *         //   undefined: no action
 *         //   false: no action
 *         //   visitor.BREAK: stop visiting altogether
 *         //   null: delete this node
 *         //   any value: replace this node with the returned value
 *       }
 *     });
 *
 * Alternatively to providing enter() and leave() functions, a visitor can
 * instead provide functions named the same as the kinds of AST nodes, or
 * enter/leave visitors at a named key, leading to four permutations of the
 * visitor API:
 *
 * 1) Named visitors triggered when entering a node of a specific kind.
 *
 *     visit(ast, {
 *       Kind(node) {
 *         // enter the "Kind" node
 *       }
 *     })
 *
 * 2) Named visitors that trigger upon entering and leaving a node of
 *    a specific kind.
 *
 *     visit(ast, {
 *       Kind: {
 *         enter(node) {
 *           // enter the "Kind" node
 *         }
 *         leave(node) {
 *           // leave the "Kind" node
 *         }
 *       }
 *     })
 *
 * 3) Generic visitors that trigger upon entering and leaving any node.
 *
 *     visit(ast, {
 *       enter(node) {
 *         // enter any node
 *       },
 *       leave(node) {
 *         // leave any node
 *       }
 *     })
 *
 * 4) Parallel visitors for entering and leaving nodes of a specific kind.
 *
 *     visit(ast, {
 *       enter: {
 *         Kind(node) {
 *           // enter the "Kind" node
 *         }
 *       },
 *       leave: {
 *         Kind(node) {
 *           // leave the "Kind" node
 *         }
 *       }
 *     })
 */
export function visit(
  root: ASTNode,
  visitor: Visitor<ASTKindToNode>,
  visitorKeys?: VisitorKeyMap<ASTKindToNode>, // default: QueryDocumentKeys
): any;

/**
 * Creates a new visitor instance which delegates to many visitors to run in
 * parallel. Each visitor will be visited for each node before moving on.
 *
 * If a prior visitor edits a node, no following visitors will see that node.
 */
export function visitInParallel(
  visitors: ReadonlyArray<Visitor<ASTKindToNode>>,
): Visitor<ASTKindToNode>;

/**
 * Given a visitor instance, if it is leaving or not, and a node kind, return
 * the function the visitor runtime should call.
 */
export function getVisitFn(
  visitor: Visitor<any>,
  kind: string,
  isLeaving: boolean,
): Maybe<VisitFn<any>>;
