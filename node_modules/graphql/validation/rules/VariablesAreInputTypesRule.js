"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.VariablesAreInputTypesRule = VariablesAreInputTypesRule;

var _GraphQLError = require("../../error/GraphQLError");

var _printer = require("../../language/printer");

var _definition = require("../../type/definition");

var _typeFromAST = require("../../utilities/typeFromAST");

/**
 * Variables are input types
 *
 * A GraphQL operation is only valid if all the variables it defines are of
 * input types (scalar, enum, or input object).
 */
function VariablesAreInputTypesRule(context) {
  return {
    VariableDefinition: function VariableDefinition(node) {
      var type = (0, _typeFromAST.typeFromAST)(context.getSchema(), node.type);

      if (type && !(0, _definition.isInputType)(type)) {
        var variableName = node.variable.name.value;
        var typeName = (0, _printer.print)(node.type);
        context.reportError(new _GraphQLError.GraphQLError("Variable \"$".concat(variableName, "\" cannot be non-input type \"").concat(typeName, "\"."), node.type));
      }
    }
  };
}
