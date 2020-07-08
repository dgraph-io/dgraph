import { print } from 'graphql/language/printer';
import gql from 'graphql-tag';
import { FragmentDefinitionNode, OperationDefinitionNode } from 'graphql';

import {
  checkDocument,
  getFragmentDefinitions,
  getQueryDefinition,
  getMutationDefinition,
  createFragmentMap,
  FragmentMap,
  getDefaultValues,
  getOperationName,
} from '../getFromAST';

describe('AST utility functions', () => {
  it('should correctly check a document for correctness', () => {
    const multipleQueries = gql`
      query {
        author {
          firstName
          lastName
        }
      }

      query {
        author {
          address
        }
      }
    `;
    expect(() => {
      checkDocument(multipleQueries);
    }).toThrow();

    const namedFragment = gql`
      query {
        author {
          ...authorDetails
        }
      }

      fragment authorDetails on Author {
        firstName
        lastName
      }
    `;
    expect(() => {
      checkDocument(namedFragment);
    }).not.toThrow();
  });

  it('should get fragment definitions from a document containing a single fragment', () => {
    const singleFragmentDefinition = gql`
      query {
        author {
          ...authorDetails
        }
      }

      fragment authorDetails on Author {
        firstName
        lastName
      }
    `;
    const expectedDoc = gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }
    `;
    const expectedResult: FragmentDefinitionNode[] = [
      expectedDoc.definitions[0] as FragmentDefinitionNode,
    ];
    const actualResult = getFragmentDefinitions(singleFragmentDefinition);
    expect(actualResult.length).toEqual(expectedResult.length);
    expect(print(actualResult[0])).toBe(print(expectedResult[0]));
  });

  it('should get fragment definitions from a document containing a multiple fragments', () => {
    const multipleFragmentDefinitions = gql`
      query {
        author {
          ...authorDetails
          ...moreAuthorDetails
        }
      }

      fragment authorDetails on Author {
        firstName
        lastName
      }

      fragment moreAuthorDetails on Author {
        address
      }
    `;
    const expectedDoc = gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }

      fragment moreAuthorDetails on Author {
        address
      }
    `;
    const expectedResult: FragmentDefinitionNode[] = [
      expectedDoc.definitions[0] as FragmentDefinitionNode,
      expectedDoc.definitions[1] as FragmentDefinitionNode,
    ];
    const actualResult = getFragmentDefinitions(multipleFragmentDefinitions);
    expect(actualResult.map(print)).toEqual(expectedResult.map(print));
  });

  it('should get the correct query definition out of a query containing multiple fragments', () => {
    const queryWithFragments = gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }

      fragment moreAuthorDetails on Author {
        address
      }

      query {
        author {
          ...authorDetails
          ...moreAuthorDetails
        }
      }
    `;
    const expectedDoc = gql`
      query {
        author {
          ...authorDetails
          ...moreAuthorDetails
        }
      }
    `;
    const expectedResult: OperationDefinitionNode = expectedDoc
      .definitions[0] as OperationDefinitionNode;
    const actualResult = getQueryDefinition(queryWithFragments);

    expect(print(actualResult)).toEqual(print(expectedResult));
  });

  it('should throw if we try to get the query definition of a document with no query', () => {
    const mutationWithFragments = gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }

      mutation {
        createAuthor(firstName: "John", lastName: "Smith") {
          ...authorDetails
        }
      }
    `;
    expect(() => {
      getQueryDefinition(mutationWithFragments);
    }).toThrow();
  });

  it('should get the correct mutation definition out of a mutation with multiple fragments', () => {
    const mutationWithFragments = gql`
      mutation {
        createAuthor(firstName: "John", lastName: "Smith") {
          ...authorDetails
        }
      }

      fragment authorDetails on Author {
        firstName
        lastName
      }
    `;
    const expectedDoc = gql`
      mutation {
        createAuthor(firstName: "John", lastName: "Smith") {
          ...authorDetails
        }
      }
    `;
    const expectedResult: OperationDefinitionNode = expectedDoc
      .definitions[0] as OperationDefinitionNode;
    const actualResult = getMutationDefinition(mutationWithFragments);
    expect(print(actualResult)).toEqual(print(expectedResult));
  });

  it('should create the fragment map correctly', () => {
    const fragments = getFragmentDefinitions(gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }

      fragment moreAuthorDetails on Author {
        address
      }
    `);
    const fragmentMap = createFragmentMap(fragments);
    const expectedTable: FragmentMap = {
      authorDetails: fragments[0],
      moreAuthorDetails: fragments[1],
    };
    expect(fragmentMap).toEqual(expectedTable);
  });

  it('should return an empty fragment map if passed undefined argument', () => {
    expect(createFragmentMap(undefined)).toEqual({});
  });

  it('should get the operation name out of a query', () => {
    const query = gql`
      query nameOfQuery {
        fortuneCookie
      }
    `;
    const operationName = getOperationName(query);
    expect(operationName).toEqual('nameOfQuery');
  });

  it('should get the operation name out of a mutation', () => {
    const query = gql`
      mutation nameOfMutation {
        fortuneCookie
      }
    `;
    const operationName = getOperationName(query);
    expect(operationName).toEqual('nameOfMutation');
  });

  it('should return null if the query does not have an operation name', () => {
    const query = gql`
      {
        fortuneCookie
      }
    `;
    const operationName = getOperationName(query);
    expect(operationName).toEqual(null);
  });

  it('should throw if type definitions found in document', () => {
    const queryWithTypeDefination = gql`
      fragment authorDetails on Author {
        firstName
        lastName
      }

      query($search: AuthorSearchInputType) {
        author(search: $search) {
          ...authorDetails
        }
      }

      input AuthorSearchInputType {
        firstName: String
      }
    `;
    expect(() => {
      getQueryDefinition(queryWithTypeDefination);
    }).toThrowError(
      'Schema type definitions not allowed in queries. Found: "InputObjectTypeDefinition"',
    );
  });

  describe('getDefaultValues', () => {
    it('will create an empty variable object if no default values are provided', () => {
      const basicQuery = gql`
        query people($first: Int, $second: String) {
          allPeople(first: $first) {
            people {
              name
            }
          }
        }
      `;

      expect(getDefaultValues(getQueryDefinition(basicQuery))).toEqual({});
    });

    it('will create a variable object based on the definition node with default values', () => {
      const basicQuery = gql`
        query people($first: Int = 1, $second: String!) {
          allPeople(first: $first) {
            people {
              name
            }
          }
        }
      `;

      const complexMutation = gql`
        mutation complexStuff(
          $test: Input = { key1: ["value", "value2"], key2: { key3: 4 } }
        ) {
          complexStuff(test: $test) {
            people {
              name
            }
          }
        }
      `;

      expect(getDefaultValues(getQueryDefinition(basicQuery))).toEqual({
        first: 1,
      });
      expect(getDefaultValues(getMutationDefinition(complexMutation))).toEqual({
        test: { key1: ['value', 'value2'], key2: { key3: 4 } },
      });
    });
  });
});
