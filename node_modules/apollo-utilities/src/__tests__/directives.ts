import gql from 'graphql-tag';
import { cloneDeep } from 'lodash';

import { shouldInclude, hasDirectives } from '../directives';
import { getQueryDefinition } from '../getFromAST';

describe('hasDirective', () => {
  it('should allow searching the ast for a directive', () => {
    const query = gql`
      query Simple {
        field @live
      }
    `;
    expect(hasDirectives(['live'], query)).toBe(true);
    expect(hasDirectives(['defer'], query)).toBe(false);
  });
  it('works for all operation types', () => {
    const query = gql`
      {
        field @live {
          subField {
            hello @live
          }
        }
      }
    `;

    const mutation = gql`
      mutation Directive {
        mutate {
          field {
            subField {
              hello @live
            }
          }
        }
      }
    `;

    const subscription = gql`
      subscription LiveDirective {
        sub {
          field {
            subField {
              hello @live
            }
          }
        }
      }
    `;

    [query, mutation, subscription].forEach(x => {
      expect(hasDirectives(['live'], x)).toBe(true);
      expect(hasDirectives(['defer'], x)).toBe(false);
    });
  });
  it('works for simple fragments', () => {
    const query = gql`
      query Simple {
        ...fieldFragment
      }

      fragment fieldFragment on Field {
        foo @live
      }
    `;
    expect(hasDirectives(['live'], query)).toBe(true);
    expect(hasDirectives(['defer'], query)).toBe(false);
  });
  it('works for nested fragments', () => {
    const query = gql`
      query Simple {
        ...fieldFragment1
      }

      fragment fieldFragment1 on Field {
        bar {
          baz {
            ...nestedFragment
          }
        }
      }

      fragment nestedFragment on Field {
        foo @live
      }
    `;
    expect(hasDirectives(['live'], query)).toBe(true);
    expect(hasDirectives(['defer'], query)).toBe(false);
  });
});

describe('shouldInclude', () => {
  it('should should not include a skipped field', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, {})).toBe(true);
  });

  it('should include an included field', () => {
    const query = gql`
      query {
        fortuneCookie @include(if: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(shouldInclude(field, {})).toBe(true);
  });

  it('should not include a not include: false field', () => {
    const query = gql`
      query {
        fortuneCookie @include(if: false)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, {})).toBe(true);
  });

  it('should include a skip: false field', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: false)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(shouldInclude(field, {})).toBe(true);
  });

  it('should not include a field if skip: true and include: true', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: true) @include(if: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, {})).toBe(true);
  });

  it('should not include a field if skip: true and include: false', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: true) @include(if: false)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, {})).toBe(true);
  });

  it('should include a field if skip: false and include: true', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: false) @include(if: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(shouldInclude(field, {})).toBe(true);
  });

  it('should not include a field if skip: false and include: false', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: false) @include(if: false)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, {})).toBe(true);
  });

  it('should leave the original query unmodified', () => {
    const query = gql`
      query {
        fortuneCookie @skip(if: false) @include(if: false)
      }
    `;
    const queryClone = cloneDeep(query);
    const field = getQueryDefinition(query).selectionSet.selections[0];
    shouldInclude(field, {});
    expect(query).toEqual(queryClone);
  });

  it('does not throw an error on an unsupported directive', () => {
    const query = gql`
      query {
        fortuneCookie @dosomething(if: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];

    expect(() => {
      shouldInclude(field, {});
    }).not.toThrow();
  });

  it('throws an error on an invalid argument for the skip directive', () => {
    const query = gql`
      query {
        fortuneCookie @skip(nothing: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];

    expect(() => {
      shouldInclude(field, {});
    }).toThrow();
  });

  it('throws an error on an invalid argument for the include directive', () => {
    const query = gql`
      query {
        fortuneCookie @include(nothing: true)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];

    expect(() => {
      shouldInclude(field, {});
    }).toThrow();
  });

  it('throws an error on an invalid variable name within a directive argument', () => {
    const query = gql`
      query {
        fortuneCookie @include(if: $neverDefined)
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(() => {
      shouldInclude(field, {});
    }).toThrow();
  });

  it('evaluates variables on skip fields', () => {
    const query = gql`
      query($shouldSkip: Boolean) {
        fortuneCookie @skip(if: $shouldSkip)
      }
    `;
    const variables = {
      shouldSkip: true,
    };
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, variables)).toBe(true);
  });

  it('evaluates variables on include fields', () => {
    const query = gql`
      query($shouldSkip: Boolean) {
        fortuneCookie @include(if: $shouldInclude)
      }
    `;
    const variables = {
      shouldInclude: false,
    };
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(!shouldInclude(field, variables)).toBe(true);
  });

  it('throws an error if the value of the argument is not a variable or boolean', () => {
    const query = gql`
      query {
        fortuneCookie @include(if: "string")
      }
    `;
    const field = getQueryDefinition(query).selectionSet.selections[0];
    expect(() => {
      shouldInclude(field, {});
    }).toThrow();
  });
});
