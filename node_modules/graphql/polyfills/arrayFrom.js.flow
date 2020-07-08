// @flow strict

import { SYMBOL_ITERATOR } from './symbols';

declare function arrayFrom<T>(arrayLike: Iterable<T>): Array<T>;
// eslint-disable-next-line no-redeclare
declare function arrayFrom<T: mixed>(
  arrayLike: mixed,
  mapFn?: (elem: mixed, index: number) => T,
  thisArg?: mixed,
): Array<T>;

/* eslint-disable no-redeclare */
// $FlowFixMe
const arrayFrom =
  Array.from ||
  function (obj, mapFn, thisArg) {
    if (obj == null) {
      throw new TypeError(
        'Array.from requires an array-like object - not null or undefined',
      );
    }

    // Is Iterable?
    const iteratorMethod = obj[SYMBOL_ITERATOR];
    if (typeof iteratorMethod === 'function') {
      const iterator = iteratorMethod.call(obj);
      const result = [];
      let step;

      for (let i = 0; !(step = iterator.next()).done; ++i) {
        result.push(mapFn.call(thisArg, step.value, i));
        // Infinite Iterators could cause forEach to run forever.
        // After a very large number of iterations, produce an error.
        // istanbul ignore if (Too big to actually test)
        if (i > 9999999) {
          throw new TypeError('Near-infinite iteration.');
        }
      }
      return result;
    }

    // Is Array like?
    const length = obj.length;
    if (typeof length === 'number' && length >= 0 && length % 1 === 0) {
      const result = [];

      for (let i = 0; i < length; ++i) {
        if (Object.prototype.hasOwnProperty.call(obj, i)) {
          result.push(mapFn.call(thisArg, obj[i], i));
        }
      }
      return result;
    }

    return [];
  };

export default arrayFrom;
