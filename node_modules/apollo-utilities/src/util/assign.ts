/**
 * Adds the properties of one or more source objects to a target object. Works exactly like
 * `Object.assign`, but as a utility to maintain support for IE 11.
 *
 * @see https://github.com/apollostack/apollo-client/pull/1009
 */
export function assign<A, B>(a: A, b: B): A & B;
export function assign<A, B, C>(a: A, b: B, c: C): A & B & C;
export function assign<A, B, C, D>(a: A, b: B, c: C, d: D): A & B & C & D;
export function assign<A, B, C, D, E>(
  a: A,
  b: B,
  c: C,
  d: D,
  e: E,
): A & B & C & D & E;
export function assign(target: any, ...sources: Array<any>): any;
export function assign(
  target: { [key: string]: any },
  ...sources: Array<{ [key: string]: any }>
): { [key: string]: any } {
  sources.forEach(source => {
    if (typeof source === 'undefined' || source === null) {
      return;
    }
    Object.keys(source).forEach(key => {
      target[key] = source[key];
    });
  });
  return target;
}
