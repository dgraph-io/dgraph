// @flow strict

import invariant from './invariant';
import nodejsCustomInspectSymbol from './nodejsCustomInspectSymbol';

/**
 * The `defineInspect()` function defines `inspect()` prototype method as alias of `toJSON`
 */
export default function defineInspect(
  classObject: Class<any> | ((...args: Array<any>) => mixed),
): void {
  const fn = classObject.prototype.toJSON;
  invariant(typeof fn === 'function');

  classObject.prototype.inspect = fn;

  // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2317')
  if (nodejsCustomInspectSymbol) {
    classObject.prototype[nodejsCustomInspectSymbol] = fn;
  }
}
