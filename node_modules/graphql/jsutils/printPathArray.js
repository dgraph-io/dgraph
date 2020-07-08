"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = printPathArray;

/**
 * Build a string describing the path.
 */
function printPathArray(path) {
  return path.map(function (key) {
    return typeof key === 'number' ? '[' + key.toString() + ']' : '.' + key;
  }).join('');
}
