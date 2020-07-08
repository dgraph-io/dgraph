"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.ScalarLeafsRule = ScalarLeafsRule;

var _inspect = _interopRequireDefault(require("../../jsutils/inspect"));

var _GraphQLError = require("../../error/GraphQLError");

var _definition = require("../../type/definition");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Scalar leafs
 *
 * A GraphQL document is valid only if all leaf fields (fields without
 * sub selections) are of scalar or enum types.
 */
function ScalarLeafsRule(context) {
  return {
    Field: function Field(node) {
      var type = context.getType();
      var selectionSet = node.selectionSet;

      if (type) {
        if ((0, _definition.isLeafType)((0, _definition.getNamedType)(type))) {
          if (selectionSet) {
            var fieldName = node.name.value;
            var typeStr = (0, _inspect.default)(type);
            context.reportError(new _GraphQLError.GraphQLError("Field \"".concat(fieldName, "\" must not have a selection since type \"").concat(typeStr, "\" has no subfields."), selectionSet));
          }
        } else if (!selectionSet) {
          var _fieldName = node.name.value;

          var _typeStr = (0, _inspect.default)(type);

          context.reportError(new _GraphQLError.GraphQLError("Field \"".concat(_fieldName, "\" of type \"").concat(_typeStr, "\" must have a selection of subfields. Did you mean \"").concat(_fieldName, " { ... }\"?"), node));
        }
      }
    }
  };
}
