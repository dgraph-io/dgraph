import {
  DocumentNode,
  OperationDefinitionNode,
  SelectionSetNode,
  FieldNode,
  FragmentDefinitionNode,
} from 'graphql';

export function queryFromPojo(obj: any): DocumentNode {
  const op: OperationDefinitionNode = {
    kind: 'OperationDefinition',
    operation: 'query',
    name: {
      kind: 'Name',
      value: 'GeneratedClientQuery',
    },
    selectionSet: selectionSetFromObj(obj),
  };

  const out: DocumentNode = {
    kind: 'Document',
    definitions: [op],
  };

  return out;
}

export function fragmentFromPojo(obj: any, typename?: string): DocumentNode {
  const frag: FragmentDefinitionNode = {
    kind: 'FragmentDefinition',
    typeCondition: {
      kind: 'NamedType',
      name: {
        kind: 'Name',
        value: typename || '__FakeType',
      },
    },
    name: {
      kind: 'Name',
      value: 'GeneratedClientQuery',
    },
    selectionSet: selectionSetFromObj(obj),
  };

  const out: DocumentNode = {
    kind: 'Document',
    definitions: [frag],
  };

  return out;
}

function selectionSetFromObj(obj: any): SelectionSetNode {
  if (
    typeof obj === 'number' ||
    typeof obj === 'boolean' ||
    typeof obj === 'string' ||
    typeof obj === 'undefined' ||
    obj === null
  ) {
    // No selection set here
    return null;
  }

  if (Array.isArray(obj)) {
    // GraphQL queries don't include arrays
    return selectionSetFromObj(obj[0]);
  }

  // Now we know it's an object
  const selections: FieldNode[] = [];

  Object.keys(obj).forEach(key => {
    const nestedSelSet: SelectionSetNode = selectionSetFromObj(obj[key]);

    const field: FieldNode = {
      kind: 'Field',
      name: {
        kind: 'Name',
        value: key,
      },
      selectionSet: nestedSelSet || undefined,
    };

    selections.push(field);
  });

  const selectionSet: SelectionSetNode = {
    kind: 'SelectionSet',
    selections,
  };

  return selectionSet;
}

export const justTypenameQuery: DocumentNode = {
  kind: 'Document',
  definitions: [
    {
      kind: 'OperationDefinition',
      operation: 'query',
      name: null,
      variableDefinitions: null,
      directives: [],
      selectionSet: {
        kind: 'SelectionSet',
        selections: [
          {
            kind: 'Field',
            alias: null,
            name: {
              kind: 'Name',
              value: '__typename',
            },
            arguments: [],
            directives: [],
            selectionSet: null,
          },
        ],
      },
    },
  ],
};
