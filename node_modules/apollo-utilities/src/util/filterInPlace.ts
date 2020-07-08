export function filterInPlace<T>(
  array: T[],
  test: (elem: T) => boolean,
  context?: any,
): T[] {
  let target = 0;
  array.forEach(function (elem, i) {
    if (test.call(this, elem, i, array)) {
      array[target++] = elem;
    }
  }, context);
  array.length = target;
  return array;
}
