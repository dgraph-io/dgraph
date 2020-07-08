"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.VariablesInAllowedPositionRule = VariablesInAllowedPositionRule;

var _inspect = _interopRequireDefault(require("../../jsutils/inspect"));

var _GraphQLError = require("../../error/GraphQLError");

var _kinds = require("../../language/kinds");

var _definition = require("../../type/definition");

var _typeFromAST = require("../../utilities/typeFromAST");

var _typeComparators = require("../../utilities/typeComparators");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Variables passed to field arguments conform to type
 */
function VariablesInAllowedPositionRule(context) {
  var varDefMap = Object.create(null);
  return {
    OperationDefinition: {
      enter: function enter() {
        varDefMap = Object.create(null);
      },
      leave: function leave(operation) {
        var usages = context.getRecursiveVariableUsages(operation);

        for (var _i2 = 0; _i2 < usages.length; _i2++) {
          var _ref2 = usages[_i2];
          var node = _ref2.node;
          var type = _ref2.type;
          var defaultValue = _ref2.defaultValue;
          var varName = node.name.value;
          var varDef = varDefMap[varName];

          if (varDef && type) {
            // A var type is allowed if it is the same or more strict (e.g. is
            // a subtype of) than the expected type. It can be more strict if
            // the variable type is non-null when the expected type is nullable.
            // If both are list types, the variable item type can be more strict
            // than the expected item type (contravariant).
            var schema = context.getSchema();
            var varType = (0, _typeFromAST.typeFromAST)(schema, varDef.type);

            if (varType && !allowedVariableUsage(schema, varType, varDef.defaultValue, type, defaultValue)) {
              var varTypeStr = (0, _inspect.default)(varType);
              var typeStr = (0, _inspect.default)(type);
              context.reportError(new _GraphQLError.GraphQLError("Variable \"$".concat(varName, "\" of type \"").concat(varTypeStr, "\" used in position expecting type \"").concat(typeStr, "\"."), [varDef, node]));
            }
          }
        }
      }
    },
    VariableDefinition: function VariableDefinition(node) {
      varDefMap[node.variable.name.value] = node;
    }
  };
}
/**
 * Returns true if the variable is allowed in the location it was found,
 * which includes considering if default values exist for either the variable
 * or the location at which it is located.
 */


function allowedVariableUsage(schema, varType, varDefaultValue, locationType, locationDefaultValue) {
  if ((0, _definition.isNonNullType)(locationType) && !(0, _definition.isNonNullType)(varType)) {
    var hasNonNullVariableDefaultValue = varDefaultValue != null && varDefaultValue.kind !== _kinds.Kind.NULL;
    var hasLocationDefaultValue = locationDefaultValue !== undefined;

    if (!hasNonNullVariableDefaultValue && !hasLocationDefaultValue) {
      return false;
    }

    var nullableLocationType = locationType.ofType;
    return (0, _typeComparators.isTypeSubTypeOf)(schema, varType, nullableLocationType);
  }

  return (0, _typeComparators.isTypeSubTypeOf)(schema, varType, locationType);
}
