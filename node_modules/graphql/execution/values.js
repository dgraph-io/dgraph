"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.getVariableValues = getVariableValues;
exports.getArgumentValues = getArgumentValues;
exports.getDirectiveValues = getDirectiveValues;

var _find = _interopRequireDefault(require("../polyfills/find"));

var _keyMap = _interopRequireDefault(require("../jsutils/keyMap"));

var _inspect = _interopRequireDefault(require("../jsutils/inspect"));

var _printPathArray = _interopRequireDefault(require("../jsutils/printPathArray"));

var _GraphQLError = require("../error/GraphQLError");

var _kinds = require("../language/kinds");

var _printer = require("../language/printer");

var _definition = require("../type/definition");

var _typeFromAST = require("../utilities/typeFromAST");

var _valueFromAST = require("../utilities/valueFromAST");

var _coerceInputValue = require("../utilities/coerceInputValue");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Prepares an object map of variableValues of the correct type based on the
 * provided variable definitions and arbitrary input. If the input cannot be
 * parsed to match the variable definitions, a GraphQLError will be thrown.
 *
 * Note: The returned value is a plain Object with a prototype, since it is
 * exposed to user code. Care should be taken to not pull values from the
 * Object prototype.
 *
 * @internal
 */
function getVariableValues(schema, varDefNodes, inputs, options) {
  var errors = [];
  var maxErrors = options === null || options === void 0 ? void 0 : options.maxErrors;

  try {
    var coerced = coerceVariableValues(schema, varDefNodes, inputs, function (error) {
      if (maxErrors != null && errors.length >= maxErrors) {
        throw new _GraphQLError.GraphQLError('Too many errors processing variables, error limit reached. Execution aborted.');
      }

      errors.push(error);
    });

    if (errors.length === 0) {
      return {
        coerced: coerced
      };
    }
  } catch (error) {
    errors.push(error);
  }

  return {
    errors: errors
  };
}

function coerceVariableValues(schema, varDefNodes, inputs, onError) {
  var coercedValues = {};

  var _loop = function _loop(_i2) {
    var varDefNode = varDefNodes[_i2];
    var varName = varDefNode.variable.name.value;
    var varType = (0, _typeFromAST.typeFromAST)(schema, varDefNode.type);

    if (!(0, _definition.isInputType)(varType)) {
      // Must use input types for variables. This should be caught during
      // validation, however is checked again here for safety.
      var varTypeStr = (0, _printer.print)(varDefNode.type);
      onError(new _GraphQLError.GraphQLError("Variable \"$".concat(varName, "\" expected value of type \"").concat(varTypeStr, "\" which cannot be used as an input type."), varDefNode.type));
      return "continue";
    }

    if (!hasOwnProperty(inputs, varName)) {
      if (varDefNode.defaultValue) {
        coercedValues[varName] = (0, _valueFromAST.valueFromAST)(varDefNode.defaultValue, varType);
      } else if ((0, _definition.isNonNullType)(varType)) {
        var _varTypeStr = (0, _inspect.default)(varType);

        onError(new _GraphQLError.GraphQLError("Variable \"$".concat(varName, "\" of required type \"").concat(_varTypeStr, "\" was not provided."), varDefNode));
      }

      return "continue";
    }

    var value = inputs[varName];

    if (value === null && (0, _definition.isNonNullType)(varType)) {
      var _varTypeStr2 = (0, _inspect.default)(varType);

      onError(new _GraphQLError.GraphQLError("Variable \"$".concat(varName, "\" of non-null type \"").concat(_varTypeStr2, "\" must not be null."), varDefNode));
      return "continue";
    }

    coercedValues[varName] = (0, _coerceInputValue.coerceInputValue)(value, varType, function (path, invalidValue, error) {
      var prefix = "Variable \"$".concat(varName, "\" got invalid value ") + (0, _inspect.default)(invalidValue);

      if (path.length > 0) {
        prefix += " at \"".concat(varName).concat((0, _printPathArray.default)(path), "\"");
      }

      onError(new _GraphQLError.GraphQLError(prefix + '; ' + error.message, varDefNode, undefined, undefined, undefined, error.originalError));
    });
  };

  for (var _i2 = 0; _i2 < varDefNodes.length; _i2++) {
    var _ret = _loop(_i2);

    if (_ret === "continue") continue;
  }

  return coercedValues;
}
/**
 * Prepares an object map of argument values given a list of argument
 * definitions and list of argument AST nodes.
 *
 * Note: The returned value is a plain Object with a prototype, since it is
 * exposed to user code. Care should be taken to not pull values from the
 * Object prototype.
 *
 * @internal
 */


function getArgumentValues(def, node, variableValues) {
  var _node$arguments;

  var coercedValues = {}; // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')

  var argumentNodes = (_node$arguments = node.arguments) !== null && _node$arguments !== void 0 ? _node$arguments : [];
  var argNodeMap = (0, _keyMap.default)(argumentNodes, function (arg) {
    return arg.name.value;
  });

  for (var _i4 = 0, _def$args2 = def.args; _i4 < _def$args2.length; _i4++) {
    var argDef = _def$args2[_i4];
    var name = argDef.name;
    var argType = argDef.type;
    var argumentNode = argNodeMap[name];

    if (!argumentNode) {
      if (argDef.defaultValue !== undefined) {
        coercedValues[name] = argDef.defaultValue;
      } else if ((0, _definition.isNonNullType)(argType)) {
        throw new _GraphQLError.GraphQLError("Argument \"".concat(name, "\" of required type \"").concat((0, _inspect.default)(argType), "\" ") + 'was not provided.', node);
      }

      continue;
    }

    var valueNode = argumentNode.value;
    var isNull = valueNode.kind === _kinds.Kind.NULL;

    if (valueNode.kind === _kinds.Kind.VARIABLE) {
      var variableName = valueNode.name.value;

      if (variableValues == null || !hasOwnProperty(variableValues, variableName)) {
        if (argDef.defaultValue !== undefined) {
          coercedValues[name] = argDef.defaultValue;
        } else if ((0, _definition.isNonNullType)(argType)) {
          throw new _GraphQLError.GraphQLError("Argument \"".concat(name, "\" of required type \"").concat((0, _inspect.default)(argType), "\" ") + "was provided the variable \"$".concat(variableName, "\" which was not provided a runtime value."), valueNode);
        }

        continue;
      }

      isNull = variableValues[variableName] == null;
    }

    if (isNull && (0, _definition.isNonNullType)(argType)) {
      throw new _GraphQLError.GraphQLError("Argument \"".concat(name, "\" of non-null type \"").concat((0, _inspect.default)(argType), "\" ") + 'must not be null.', valueNode);
    }

    var coercedValue = (0, _valueFromAST.valueFromAST)(valueNode, argType, variableValues);

    if (coercedValue === undefined) {
      // Note: ValuesOfCorrectTypeRule validation should catch this before
      // execution. This is a runtime check to ensure execution does not
      // continue with an invalid argument value.
      throw new _GraphQLError.GraphQLError("Argument \"".concat(name, "\" has invalid value ").concat((0, _printer.print)(valueNode), "."), valueNode);
    }

    coercedValues[name] = coercedValue;
  }

  return coercedValues;
}
/**
 * Prepares an object map of argument values given a directive definition
 * and a AST node which may contain directives. Optionally also accepts a map
 * of variable values.
 *
 * If the directive does not exist on the node, returns undefined.
 *
 * Note: The returned value is a plain Object with a prototype, since it is
 * exposed to user code. Care should be taken to not pull values from the
 * Object prototype.
 */


function getDirectiveValues(directiveDef, node, variableValues) {
  var directiveNode = node.directives && (0, _find.default)(node.directives, function (directive) {
    return directive.name.value === directiveDef.name;
  });

  if (directiveNode) {
    return getArgumentValues(directiveDef, directiveNode, variableValues);
  }
}

function hasOwnProperty(obj, prop) {
  return Object.prototype.hasOwnProperty.call(obj, prop);
}
