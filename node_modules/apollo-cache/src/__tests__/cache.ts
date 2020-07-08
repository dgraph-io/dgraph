import gql from 'graphql-tag';

import { ApolloCache as Cache } from '../cache';

class TestCache extends Cache {}

describe('abstract cache', () => {
  describe('transformDocument', () => {
    it('returns the document', () => {
      const test = new TestCache();
      expect(test.transformDocument('a')).toBe('a');
    });
  });

  describe('transformForLink', () => {
    it('returns the document', () => {
      const test = new TestCache();
      expect(test.transformForLink('a')).toBe('a');
    });
  });

  describe('readQuery', () => {
    it('runs the read method', () => {
      const test = new TestCache();
      test.read = jest.fn();

      test.readQuery({});
      expect(test.read).toBeCalled();
    });

    it('defaults optimistic to false', () => {
      const test = new TestCache();
      test.read = ({ optimistic }) => optimistic;

      expect(test.readQuery({})).toBe(false);
      expect(test.readQuery({}, true)).toBe(true);
    });
  });

  describe('readFragment', () => {
    it('runs the read method', () => {
      const test = new TestCache();
      test.read = jest.fn();
      const fragment = {
        id: 'frag',
        fragment: gql`
          fragment a on b {
            name
          }
        `,
      };

      test.readFragment(fragment);
      expect(test.read).toBeCalled();
    });

    it('defaults optimistic to false', () => {
      const test = new TestCache();
      test.read = ({ optimistic }) => optimistic;
      const fragment = {
        id: 'frag',
        fragment: gql`
          fragment a on b {
            name
          }
        `,
      };

      expect(test.readFragment(fragment)).toBe(false);
      expect(test.readFragment(fragment, true)).toBe(true);
    });
  });

  describe('writeQuery', () => {
    it('runs the write method', () => {
      const test = new TestCache();
      test.write = jest.fn();

      test.writeQuery({});
      expect(test.write).toBeCalled();
    });
  });

  describe('writeFragment', () => {
    it('runs the write method', () => {
      const test = new TestCache();
      test.write = jest.fn();
      const fragment = {
        id: 'frag',
        fragment: gql`
          fragment a on b {
            name
          }
        `,
      };

      test.writeFragment(fragment);
      expect(test.write).toBeCalled();
    });
  });

  describe('writeData', () => {
    it('either writes a fragment or a query', () => {
      const test = new TestCache();
      test.read = jest.fn();
      test.writeFragment = jest.fn();
      test.writeQuery = jest.fn();

      test.writeData({});
      expect(test.writeQuery).toBeCalled();

      test.writeData({ id: 1 });
      expect(test.read).toBeCalled();
      expect(test.writeFragment).toBeCalled();

      // Edge case for falsey id
      test.writeData({ id: 0 });
      expect(test.read).toHaveBeenCalledTimes(2);
      expect(test.writeFragment).toHaveBeenCalledTimes(2);
    });

    it('suppresses read errors', () => {
      const test = new TestCache();
      test.read = () => {
        throw new Error();
      };
      test.writeFragment = jest.fn();

      expect(() => test.writeData({ id: 1 })).not.toThrow();
      expect(test.writeFragment).toBeCalled();
    });

    it('reads __typename from typenameResult or defaults to __ClientData', () => {
      const test = new TestCache();
      test.read = () => ({ __typename: 'a' });
      let res;
      test.writeFragment = obj =>
        (res = obj.fragment.definitions[0].typeCondition.name.value);

      test.writeData({ id: 1 });
      expect(res).toBe('a');

      test.read = () => ({});

      test.writeData({ id: 1 });
      expect(res).toBe('__ClientData');
    });
  });
});
