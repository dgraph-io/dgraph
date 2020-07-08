import {
  DocumentNode,
  OperationDefinitionNode,
  FragmentDefinitionNode,
  ValueNode,
} from 'graphql';

import { invariant, InvariantError } from 'ts-invariant';

import { assign } from './util/assign';

import { valueToObjectRepresentation, JsonValue } from './storeUtils';

export function getMutationDefinition(
  doc: DocumentNode,
): OperationDefinitionNode {
  checkDocument(doc);

  let mutationDef: OperationDefinitionNode | null = doc.definitions.filter(
    definition =>
      definition.kind === 'OperationDefinition' &&
      definition.operation === 'mutation',
  )[0] as OperationDefinitionNode;

  invariant(mutationDef, 'Must contain a mutation definition.');

  return mutationDef;
}

// Checks the document for errors and throws an exception if there is an error.
export function checkDocument(doc: DocumentNode) {
  invariant(
    doc && doc.kind === 'Document',
    `Expecting a parsed GraphQL document. Perhaps you need to wrap the query \
string in a "gql" tag? http://docs.apollostack.com/apollo-client/core.html#gql`,
  );

  const operations = doc.definitions
    .filter(d => d.kind !== 'FragmentDefinition')
    .map(definition => {
      if (definition.kind !== 'OperationDefinition') {
        throw new InvariantError(
          `Schema type definitions not allowed in queries. Found: "${
            definition.kind
          }"`,
        );
      }
      return definition;
    });

  invariant(
    operations.length <= 1,
    `Ambiguous GraphQL document: contains ${operations.length} operations`,
  );

  return doc;
}

export function getOperationDefinition(
  doc: DocumentNode,
): OperationDefinitionNode | undefined {
  checkDocument(doc);
  return doc.definitions.filter(
    definition => definition.kind === 'OperationDefinition',
  )[0] as OperationDefinitionNode;
}

export function getOperationDefinitionOrDie(
  document: DocumentNode,
): OperationDefinitionNode {
  const def = getOperationDefinition(document);
  invariant(def, `GraphQL document is missing an operation`);
  return def;
}

export function getOperationName(doc: DocumentNode): string | null {
  return (
    doc.definitions
      .filter(
        definition =>
          definition.kind === 'OperationDefinition' && definition.name,
      )
      .map((x: OperationDefinitionNode) => x.name.value)[0] || null
  );
}

// Returns the FragmentDefinitions from a particular document as an array
export function getFragmentDefinitions(
  doc: DocumentNode,
): FragmentDefinitionNode[] {
  return doc.definitions.filter(
    definition => definition.kind === 'FragmentDefinition',
  ) as FragmentDefinitionNode[];
}

export function getQueryDefinition(doc: DocumentNode): OperationDefinitionNode {
  const queryDef = getOperationDefinition(doc) as OperationDefinitionNode;

  invariant(
    queryDef && queryDef.operation === 'query',
    'Must contain a query definition.',
  );

  return queryDef;
}

export function getFragmentDefinition(
  doc: DocumentNode,
): FragmentDefinitionNode {
  invariant(
    doc.kind === 'Document',
    `Expecting a parsed GraphQL document. Perhaps you need to wrap the query \
string in a "gql" tag? http://docs.apollostack.com/apollo-client/core.html#gql`,
  );

  invariant(
    doc.definitions.length <= 1,
    'Fragment must have exactly one definition.',
  );

  const fragmentDef = doc.definitions[0] as FragmentDefinitionNode;

  invariant(
    fragmentDef.kind === 'FragmentDefinition',
    'Must be a fragment definition.',
  );

  return fragmentDef as FragmentDefinitionNode;
}

/**
 * Returns the first operation definition found in this document.
 * If no operation definition is found, the first fragment definition will be returned.
 * If no definitions are found, an error will be thrown.
 */
export function getMainDefinition(
  queryDoc: DocumentNode,
): OperationDefinitionNode | FragmentDefinitionNode {
  checkDocument(queryDoc);

  let fragmentDefinition;

  for (let definition of queryDoc.definitions) {
    if (definition.kind === 'OperationDefinition') {
      const operation = (definition as OperationDefinitionNode).operation;
      if (
        operation === 'query' ||
        operation === 'mutation' ||
        operation === 'subscription'
      ) {
        return definition as OperationDefinitionNode;
      }
    }
    if (definition.kind === 'FragmentDefinition' && !fragmentDefinition) {
      // we do this because we want to allow multiple fragment definitions
      // to precede an operation definition.
      fragmentDefinition = definition as FragmentDefinitionNode;
    }
  }

  if (fragmentDefinition) {
    return fragmentDefinition;
  }

  throw new InvariantError(
    'Expected a parsed GraphQL query with a query, mutation, subscription, or a fragment.',
  );
}

/**
 * This is an interface that describes a map from fragment names to fragment definitions.
 */
export interface FragmentMap {
  [fragmentName: string]: FragmentDefinitionNode;
}

// Utility function that takes a list of fragment definitions and makes a hash out of them
// that maps the name of the fragment to the fragment definition.
export function createFragmentMap(
  fragments: FragmentDefinitionNode[] = [],
): FragmentMap {
  const symTable: FragmentMap = {};
  fragments.forEach(fragment => {
    symTable[fragment.name.value] = fragment;
  });

  return symTable;
}

export function getDefaultValues(
  definition: OperationDefinitionNode | undefined,
): { [key: string]: JsonValue } {
  if (
    definition &&
    definition.variableDefinitions &&
    definition.variableDefinitions.length
  ) {
    const defaultValues = definition.variableDefinitions
      .filter(({ defaultValue }) => defaultValue)
      .map(
        ({ variable, defaultValue }): { [key: string]: JsonValue } => {
          const defaultValueObj: { [key: string]: JsonValue } = {};
          valueToObjectRepresentation(
            defaultValueObj,
            variable.name,
            defaultValue as ValueNode,
          );

          return defaultValueObj;
        },
      );

    return assign({}, ...defaultValues);
  }

  return {};
}

/**
 * Returns the names of all variables declared by the operation.
 */
export function variablesInOperation(
  operation: OperationDefinitionNode,
): Set<string> {
  const names = new Set<string>();
  if (operation.variableDefinitions) {
    for (const definition of operation.variableDefinitions) {
      names.add(definition.variable.name.value);
    }
  }

  return names;
}
