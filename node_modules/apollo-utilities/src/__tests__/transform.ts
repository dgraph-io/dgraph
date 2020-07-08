import { print } from 'graphql/language/printer';
import gql from 'graphql-tag';
import { disableFragmentWarnings } from 'graphql-tag';

// Turn off warnings for repeated fragment names
disableFragmentWarnings();

import {
  addTypenameToDocument,
  removeDirectivesFromDocument,
  getDirectivesFromDocument,
  removeConnectionDirectiveFromDocument,
  removeArgumentsFromDocument,
  removeFragmentSpreadFromDocument,
  removeClientSetsFromDocument,
} from '../transform';
import { getQueryDefinition } from '../getFromAST';

describe('removeArgumentsFromDocument', () => {
  it('should remove a single variable', () => {
    const query = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable) {
          child
          foo
        }
        network
      }
    `;
    const expected = gql`
      query Simple {
        field {
          child
          foo
        }
        network
      }
    `;
    const doc = removeArgumentsFromDocument([{ name: 'variable' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove a single variable and the field from the query', () => {
    const query = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable) {
          child
          foo
        }
        network
      }
    `;
    const expected = gql`
      query Simple {
        network
      }
    `;
    const doc = removeArgumentsFromDocument(
      [{ name: 'variable', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });
});
describe('removeFragmentSpreadFromDocument', () => {
  it('should remove a named fragment spread', () => {
    const query = gql`
      query Simple {
        ...FragmentSpread
        property
        ...ValidSpread
      }

      fragment FragmentSpread on Thing {
        foo
        bar
        baz
      }

      fragment ValidSpread on Thing {
        oof
        rab
        zab
      }
    `;
    const expected = gql`
      query Simple {
        property
        ...ValidSpread
      }

      fragment ValidSpread on Thing {
        oof
        rab
        zab
      }
    `;
    const doc = removeFragmentSpreadFromDocument(
      [{ name: 'FragmentSpread', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });
});
describe('removeDirectivesFromDocument', () => {
  it('should not remove unused variable definitions unless the field is removed', () => {
    const query = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable) @client
        networkField
      }
    `;

    const expected = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable)
        networkField
      }
    `;

    const doc = removeDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove unused variable definitions associated with the removed directive', () => {
    const query = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable) @client
        networkField
      }
    `;

    const expected = gql`
      query Simple {
        networkField
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'client', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should not remove used variable definitions', () => {
    const query = gql`
      query Simple($variable: String!) {
        field(usingVariable: $variable) @client
        networkField(usingVariable: $variable)
      }
    `;

    const expected = gql`
      query Simple($variable: String!) {
        networkField(usingVariable: $variable)
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'client', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove fragment spreads and definitions associated with the removed directive', () => {
    const query = gql`
      query Simple {
        networkField
        field @client {
          ...ClientFragment
        }
      }

      fragment ClientFragment on Thing {
        otherField
        bar
      }
    `;

    const expected = gql`
      query Simple {
        networkField
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'client', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should not remove fragment spreads and definitions used without the removed directive', () => {
    const query = gql`
      query Simple {
        networkField {
          ...ClientFragment
        }
        field @client {
          ...ClientFragment
        }
      }

      fragment ClientFragment on Thing {
        otherField
        bar
      }
    `;

    const expected = gql`
      query Simple {
        networkField {
          ...ClientFragment
        }
      }

      fragment ClientFragment on Thing {
        otherField
        bar
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'client', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove a simple directive', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        field
      }
    `;
    const doc = removeDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove a simple directive [test function]', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        field
      }
    `;
    const test = ({ name: { value } }: { name: any }) => value === 'storage';
    const doc = removeDirectivesFromDocument([{ test }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove only the wanted directive', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        maybe @skip(if: false)
        field
      }
    `;
    const doc = removeDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove only the wanted directive [test function]', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        maybe @skip(if: false)
        field
      }
    `;
    const test = ({ name: { value } }: { name: any }) => value === 'storage';
    const doc = removeDirectivesFromDocument([{ test }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove multiple directives in the query', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
        other: field @storage
      }
    `;

    const expected = gql`
      query Simple {
        field
        other: field
      }
    `;
    const doc = removeDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove multiple directives of different kinds in the query', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
        other: field @client
      }
    `;

    const expected = gql`
      query Simple {
        maybe @skip(if: false)
        field
        other: field
      }
    `;
    const removed = [
      { name: 'storage' },
      {
        test: (directive: any) => directive.name.value === 'client',
      },
    ];
    const doc = removeDirectivesFromDocument(removed, query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove a simple directive and its field if needed', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
        keep
      }
    `;

    const expected = gql`
      query Simple {
        keep
      }
    `;
    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove a simple directive [test function]', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
        keep
      }
    `;

    const expected = gql`
      query Simple {
        keep
      }
    `;
    const test = ({ name: { value } }: { name: any }) => value === 'storage';
    const doc = removeDirectivesFromDocument([{ test, remove: true }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should return null if the query is no longer valid', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );

    expect(doc).toBe(null);
  });

  it('should return null if the query is no longer valid [test function]', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const test = ({ name: { value } }: { name: any }) => value === 'storage';
    const doc = removeDirectivesFromDocument([{ test, remove: true }], query);
    expect(doc).toBe(null);
  });

  it('should return null only if the query is not valid', () => {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );

    expect(print(doc)).toBe(print(query));
  });

  it('should return null only if the query is not valid through nested fragments', () => {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        ...inDirection
      }

      fragment inDirection on Thing {
        field @storage
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );

    expect(doc).toBe(null);
  });

  it('should only remove values asked through nested fragments', () => {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        ...inDirection
      }

      fragment inDirection on Thing {
        field @storage
        bar
      }
    `;

    const expectedQuery = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        ...inDirection
      }

      fragment inDirection on Thing {
        bar
      }
    `;
    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );

    expect(print(doc)).toBe(print(expectedQuery));
  });

  it('should return null even through fragments if needed', () => {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @storage
      }
    `;

    const doc = removeDirectivesFromDocument(
      [{ name: 'storage', remove: true }],
      query,
    );

    expect(doc).toBe(null);
  });

  it('should not throw in combination with addTypenameToDocument', () => {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        ...inDirection
      }

      fragment inDirection on Thing {
        field @storage
      }
    `;

    expect(() => {
      removeDirectivesFromDocument(
        [{ name: 'storage', remove: true }],
        addTypenameToDocument(query),
      );
    }).not.toThrow();
  });
});

describe('query transforms', () => {
  it('should correctly add typenames', () => {
    let testQuery = gql`
      query {
        author {
          name {
            firstName
            lastName
          }
        }
      }
    `;
    const newQueryDoc = addTypenameToDocument(testQuery);

    const expectedQuery = gql`
      query {
        author {
          name {
            firstName
            lastName
            __typename
          }
          __typename
        }
      }
    `;
    const expectedQueryStr = print(expectedQuery);

    expect(print(newQueryDoc)).toBe(expectedQueryStr);
  });

  it('should not add duplicates', () => {
    let testQuery = gql`
      query {
        author {
          name {
            firstName
            lastName
            __typename
          }
        }
      }
    `;
    const newQueryDoc = addTypenameToDocument(testQuery);

    const expectedQuery = gql`
      query {
        author {
          name {
            firstName
            lastName
            __typename
          }
          __typename
        }
      }
    `;
    const expectedQueryStr = print(expectedQuery);

    expect(print(newQueryDoc)).toBe(expectedQueryStr);
  });

  it('should not screw up on a FragmentSpread within the query AST', () => {
    const testQuery = gql`
      query withFragments {
        user(id: 4) {
          friends(first: 10) {
            ...friendFields
          }
        }
      }
    `;
    const expectedQuery = getQueryDefinition(gql`
      query withFragments {
        user(id: 4) {
          friends(first: 10) {
            ...friendFields
            __typename
          }
          __typename
        }
      }
    `);
    const modifiedQuery = addTypenameToDocument(testQuery);
    expect(print(expectedQuery)).toBe(print(getQueryDefinition(modifiedQuery)));
  });

  it('should modify all definitions in a document', () => {
    const testQuery = gql`
      query withFragments {
        user(id: 4) {
          friends(first: 10) {
            ...friendFields
          }
        }
      }

      fragment friendFields on User {
        firstName
        lastName
      }
    `;

    const newQueryDoc = addTypenameToDocument(testQuery);

    const expectedQuery = gql`
      query withFragments {
        user(id: 4) {
          friends(first: 10) {
            ...friendFields
            __typename
          }
          __typename
        }
      }

      fragment friendFields on User {
        firstName
        lastName
        __typename
      }
    `;

    expect(print(expectedQuery)).toBe(print(newQueryDoc));
  });

  it('should be able to apply a QueryTransformer correctly', () => {
    const testQuery = gql`
      query {
        author {
          firstName
          lastName
        }
      }
    `;

    const expectedQuery = getQueryDefinition(gql`
      query {
        author {
          firstName
          lastName
          __typename
        }
      }
    `);

    const modifiedQuery = addTypenameToDocument(testQuery);
    expect(print(expectedQuery)).toBe(print(getQueryDefinition(modifiedQuery)));
  });

  it('should be able to apply a MutationTransformer correctly', () => {
    const testQuery = gql`
      mutation {
        createAuthor(firstName: "John", lastName: "Smith") {
          firstName
          lastName
        }
      }
    `;
    const expectedQuery = gql`
      mutation {
        createAuthor(firstName: "John", lastName: "Smith") {
          firstName
          lastName
          __typename
        }
      }
    `;

    const modifiedQuery = addTypenameToDocument(testQuery);
    expect(print(expectedQuery)).toBe(print(modifiedQuery));
  });

  it('should add typename fields correctly on this one query', () => {
    const testQuery = gql`
      query Feed($type: FeedType!) {
        # Eventually move this into a no fetch query right on the entry
        # since we literally just need this info to determine whether to
        # show upvote/downvote buttons
        currentUser {
          login
        }
        feed(type: $type) {
          createdAt
          score
          commentCount
          id
          postedBy {
            login
            html_url
          }
          repository {
            name
            full_name
            description
            html_url
            stargazers_count
            open_issues_count
            created_at
            owner {
              avatar_url
            }
          }
        }
      }
    `;
    const expectedQuery = getQueryDefinition(gql`
      query Feed($type: FeedType!) {
        currentUser {
          login
          __typename
        }
        feed(type: $type) {
          createdAt
          score
          commentCount
          id
          postedBy {
            login
            html_url
            __typename
          }
          repository {
            name
            full_name
            description
            html_url
            stargazers_count
            open_issues_count
            created_at
            owner {
              avatar_url
              __typename
            }
            __typename
          }
          __typename
        }
      }
    `);
    const modifiedQuery = addTypenameToDocument(testQuery);
    expect(print(expectedQuery)).toBe(print(getQueryDefinition(modifiedQuery)));
  });

  it('should correctly remove connections', () => {
    let testQuery = gql`
      query {
        author {
          name @connection(key: "foo") {
            firstName
            lastName
          }
        }
      }
    `;
    const newQueryDoc = removeConnectionDirectiveFromDocument(testQuery);

    const expectedQuery = gql`
      query {
        author {
          name {
            firstName
            lastName
          }
        }
      }
    `;
    const expectedQueryStr = print(expectedQuery);

    expect(expectedQueryStr).toBe(print(newQueryDoc));
  });
});

describe('getDirectivesFromDocument', () => {
  it('should get query with fields of storage directive ', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with fields of storage directive [test function] ', () => {
    const query = gql`
      query Simple {
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
      }
    `;
    const test = ({ name: { value } }: { name: any }) => value === 'storage';
    const doc = getDirectivesFromDocument([{ test }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should only get query with fields of storage directive ', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should only get query with multiple fields of storage directive ', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
        other @storage
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
        other @storage
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'storage' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with fields of both storage and client directives ', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
        user @client
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
        user @client
      }
    `;
    const doc = getDirectivesFromDocument(
      [{ name: 'storage' }, { name: 'client' }],
      query,
    );
    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with different types of directive matchers ', () => {
    const query = gql`
      query Simple {
        maybe @skip(if: false)
        field @storage(if: true)
        user @client
      }
    `;

    const expected = gql`
      query Simple {
        field @storage(if: true)
        user @client
      }
    `;
    const doc = getDirectivesFromDocument(
      [
        { name: 'storage' },
        { test: directive => directive.name.value === 'client' },
      ],
      query,
    );

    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with nested fields ', () => {
    const query = gql`
      query Simple {
        user {
          firstName @client
          email
        }
      }
    `;

    const expected = gql`
      query Simple {
        user {
          firstName @client
        }
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should include all the nested fields of field that has client directive ', () => {
    const query = gql`
      query Simple {
        user @client {
          firstName
          email
        }
      }
    `;

    const expected = gql`
      query Simple {
        user @client {
          firstName
          email
        }
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should return null if the query is no longer valid', () => {
    const query = gql`
      query Simple {
        field
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(null);
  });

  it('should get query with client fields in fragment', function() {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
        other
      }
    `;
    const expected = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with client fields in fragment with nested fields', function() {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        user {
          firstName @client
          lastName
        }
      }
    `;
    const expected = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        user {
          firstName @client
        }
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get query with client fields in multiple fragments', function() {
    const query = gql`
      query Simple {
        ...fragmentSpread
        ...anotherFragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
        other
      }

      fragment anotherFragmentSpread on AnotherThing {
        user @client
        product
      }
    `;
    const expected = gql`
      query Simple {
        ...fragmentSpread
        ...anotherFragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
      }

      fragment anotherFragmentSpread on AnotherThing {
        user @client
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it("should return null if fragment didn't have client fields", function() {
    const query = gql`
      query Simple {
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(null));
  });

  it('should get query with client fields when both fields and fragements are mixed', function() {
    const query = gql`
      query Simple {
        user @client
        product @storage
        order
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
        other
      }
    `;
    const expected = gql`
      query Simple {
        user @client
        ...fragmentSpread
      }

      fragment fragmentSpread on Thing {
        field @client
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get mutation with client fields', () => {
    const query = gql`
      mutation {
        login @client
      }
    `;

    const expected = gql`
      mutation {
        login @client
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should get mutation fields of client only', () => {
    const query = gql`
      mutation {
        login @client
        updateUser
      }
    `;

    const expected = gql`
      mutation {
        login @client
      }
    `;
    const doc = getDirectivesFromDocument([{ name: 'client' }], query);
    expect(print(doc)).toBe(print(expected));
  });

  describe('includeAllFragments', () => {
    it('= false: should remove the values without a client in fragment', () => {
      const query = gql`
        fragment client on ClientData {
          hi @client
          bye @storage
          bar
        }

        query Mixed {
          foo @client {
            ...client
          }
          bar {
            baz
          }
        }
      `;

      const expected = gql`
        fragment client on ClientData {
          hi @client
        }

        query Mixed {
          foo @client {
            ...client
          }
        }
      `;
      const doc = getDirectivesFromDocument([{ name: 'client' }], query, false);
      expect(print(doc)).toBe(print(expected));
    });

    it('= true: should include the values without a client in fragment', () => {
      const query = gql`
        fragment client on ClientData {
          hi @client
          bye @storage
          bar
        }

        query Mixed {
          foo @client {
            ...client
          }
          bar {
            baz
          }
        }
      `;

      const expected = gql`
        fragment client on ClientData {
          hi @client
        }

        query Mixed {
          foo @client {
            ...client
          }
        }
      `;
      const doc = getDirectivesFromDocument([{ name: 'client' }], query, true);
      expect(print(doc)).toBe(print(expected));
    });
  });
});

describe('removeClientSetsFromDocument', () => {
  it('should remove @client fields from document', () => {
    const query = gql`
      query Author {
        name
        isLoggedIn @client
      }
    `;

    const expected = gql`
      query Author {
        name
      }
    `;
    const doc = removeClientSetsFromDocument(query);
    expect(print(doc)).toBe(print(expected));
  });

  it('should remove @client fields from fragments', () => {
    const query = gql`
      fragment authorInfo on Author {
        name
        isLoggedIn @client
      }
    `;

    const expected = gql`
      fragment authorInfo on Author {
        name
      }
    `;
    const doc = removeClientSetsFromDocument(query);
    expect(print(doc)).toBe(print(expected));
  });
});
