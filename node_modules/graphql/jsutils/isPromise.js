"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = isPromise;

/**
 * Returns true if the value acts like a Promise, i.e. has a "then" function,
 * otherwise returns false.
 */
// eslint-disable-next-line no-redeclare
function isPromise(value) {
  return typeof (value === null || value === void 0 ? void 0 : value.then) === 'function';
}
