import invariant from "./invariant.mjs";
import nodejsCustomInspectSymbol from "./nodejsCustomInspectSymbol.mjs";
/**
 * The `defineInspect()` function defines `inspect()` prototype method as alias of `toJSON`
 */

export default function defineInspect(classObject) {
  var fn = classObject.prototype.toJSON;
  typeof fn === 'function' || invariant(0);
  classObject.prototype.inspect = fn; // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2317')

  if (nodejsCustomInspectSymbol) {
    classObject.prototype[nodejsCustomInspectSymbol] = fn;
  }
}
