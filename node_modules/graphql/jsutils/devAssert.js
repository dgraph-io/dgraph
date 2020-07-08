"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = devAssert;

function devAssert(condition, message) {
  var booleanCondition = Boolean(condition); // istanbul ignore else (See transformation done in './resources/inlineInvariant.js')

  if (!booleanCondition) {
    throw new Error(message);
  }
}
