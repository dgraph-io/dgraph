import { print } from 'graphql/language/printer';
import gql from 'graphql-tag';
import { disableFragmentWarnings } from 'graphql-tag';

// Turn off warnings for repeated fragment names
disableFragmentWarnings();

import { getFragmentQueryDocument } from '../fragments';

describe('getFragmentQueryDocument', () => {
  it('will throw an error if there is an operation', () => {
    expect(() =>
      getFragmentQueryDocument(
        gql`
          {
            a
            b
            c
          }
        `,
      ),
    ).toThrowError(
      'Found a query operation. No operations are allowed when using a fragment as a query. Only fragments are allowed.',
    );
    expect(() =>
      getFragmentQueryDocument(
        gql`
          query {
            a
            b
            c
          }
        `,
      ),
    ).toThrowError(
      'Found a query operation. No operations are allowed when using a fragment as a query. Only fragments are allowed.',
    );
    expect(() =>
      getFragmentQueryDocument(
        gql`
          query Named {
            a
            b
            c
          }
        `,
      ),
    ).toThrowError(
      "Found a query operation named 'Named'. No operations are allowed when using a fragment as a query. Only fragments are allowed.",
    );
    expect(() =>
      getFragmentQueryDocument(
        gql`
          mutation Named {
            a
            b
            c
          }
        `,
      ),
    ).toThrowError(
      "Found a mutation operation named 'Named'. No operations are allowed when using a fragment as a query. " +
        'Only fragments are allowed.',
    );
    expect(() =>
      getFragmentQueryDocument(
        gql`
          subscription Named {
            a
            b
            c
          }
        `,
      ),
    ).toThrowError(
      "Found a subscription operation named 'Named'. No operations are allowed when using a fragment as a query. " +
        'Only fragments are allowed.',
    );
  });

  it('will throw an error if there is not exactly one fragment but no `fragmentName`', () => {
    expect(() => {
      getFragmentQueryDocument(gql`
        fragment foo on Foo {
          a
          b
          c
        }

        fragment bar on Bar {
          d
          e
          f
        }
      `);
    }).toThrowError(
      'Found 2 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.',
    );
    expect(() => {
      getFragmentQueryDocument(gql`
        fragment foo on Foo {
          a
          b
          c
        }

        fragment bar on Bar {
          d
          e
          f
        }

        fragment baz on Baz {
          g
          h
          i
        }
      `);
    }).toThrowError(
      'Found 3 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.',
    );
    expect(() => {
      getFragmentQueryDocument(gql`
        scalar Foo
      `);
    }).toThrowError(
      'Found 0 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.',
    );
  });

  it('will create a query document where the single fragment is spread in the root query', () => {
    expect(
      print(
        getFragmentQueryDocument(gql`
          fragment foo on Foo {
            a
            b
            c
          }
        `),
      ),
    ).toEqual(
      print(gql`
        {
          ...foo
        }

        fragment foo on Foo {
          a
          b
          c
        }
      `),
    );
  });

  it('will create a query document where the named fragment is spread in the root query', () => {
    expect(
      print(
        getFragmentQueryDocument(
          gql`
            fragment foo on Foo {
              a
              b
              c
            }

            fragment bar on Bar {
              d
              e
              f
              ...foo
            }

            fragment baz on Baz {
              g
              h
              i
              ...foo
              ...bar
            }
          `,
          'foo',
        ),
      ),
    ).toEqual(
      print(gql`
        {
          ...foo
        }

        fragment foo on Foo {
          a
          b
          c
        }

        fragment bar on Bar {
          d
          e
          f
          ...foo
        }

        fragment baz on Baz {
          g
          h
          i
          ...foo
          ...bar
        }
      `),
    );
    expect(
      print(
        getFragmentQueryDocument(
          gql`
            fragment foo on Foo {
              a
              b
              c
            }

            fragment bar on Bar {
              d
              e
              f
              ...foo
            }

            fragment baz on Baz {
              g
              h
              i
              ...foo
              ...bar
            }
          `,
          'bar',
        ),
      ),
    ).toEqual(
      print(gql`
        {
          ...bar
        }

        fragment foo on Foo {
          a
          b
          c
        }

        fragment bar on Bar {
          d
          e
          f
          ...foo
        }

        fragment baz on Baz {
          g
          h
          i
          ...foo
          ...bar
        }
      `),
    );
    expect(
      print(
        getFragmentQueryDocument(
          gql`
            fragment foo on Foo {
              a
              b
              c
            }

            fragment bar on Bar {
              d
              e
              f
              ...foo
            }

            fragment baz on Baz {
              g
              h
              i
              ...foo
              ...bar
            }
          `,
          'baz',
        ),
      ),
    ).toEqual(
      print(gql`
        {
          ...baz
        }

        fragment foo on Foo {
          a
          b
          c
        }

        fragment bar on Bar {
          d
          e
          f
          ...foo
        }

        fragment baz on Baz {
          g
          h
          i
          ...foo
          ...bar
        }
      `),
    );
  });
});
