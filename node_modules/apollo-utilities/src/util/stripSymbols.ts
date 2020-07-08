/**
 * In order to make assertions easier, this function strips `symbol`'s from
 * the incoming data.
 *
 * This can be handy when running tests against `apollo-client` for example,
 * since it adds `symbol`'s to the data in the store. Jest's `toEqual`
 * function now covers `symbol`'s (https://github.com/facebook/jest/pull/3437),
 * which means all test data used in a `toEqual` comparison would also have to
 * include `symbol`'s, to pass. By stripping `symbol`'s from the cache data
 * we can compare against more simplified test data.
 */
export function stripSymbols<T>(data: T): T {
  return JSON.parse(JSON.stringify(data));
}
