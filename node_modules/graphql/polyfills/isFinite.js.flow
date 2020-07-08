// @flow strict

declare function isFinitePolyfill(
  value: mixed,
): boolean %checks(typeof value === 'number');

/* eslint-disable no-redeclare */
// $FlowFixMe workaround for: https://github.com/facebook/flow/issues/4441
const isFinitePolyfill =
  Number.isFinite ||
  function (value) {
    return typeof value === 'number' && isFinite(value);
  };
export default isFinitePolyfill;
