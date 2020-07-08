/**
 * Given a Path and a key, return a new Path containing the new key.
 */
export function addPath(prev, key, typename) {
  return {
    prev: prev,
    key: key,
    typename: typename
  };
}
/**
 * Given a Path, return an Array of the path keys.
 */

export function pathToArray(path) {
  var flattened = [];
  var curr = path;

  while (curr) {
    flattened.push(curr.key);
    curr = curr.prev;
  }

  return flattened.reverse();
}
