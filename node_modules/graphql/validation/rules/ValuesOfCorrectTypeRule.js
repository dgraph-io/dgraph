"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.ValuesOfCorrectTypeRule = ValuesOfCorrectTypeRule;

var _objectValues3 = _interopRequireDefault(require("../../polyfills/objectValues"));

var _keyMap = _interopRequireDefault(require("../../jsutils/keyMap"));

var _inspect = _interopRequireDefault(require("../../jsutils/inspect"));

var _didYouMean = _interopRequireDefault(require("../../jsutils/didYouMean"));

var _suggestionList = _interopRequireDefault(require("../../jsutils/suggestionList"));

var _GraphQLError = require("../../error/GraphQLError");

var _printer = require("../../language/printer");

var _definition = require("../../type/definition");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Value literals of correct type
 *
 * A GraphQL document is only valid if all value literals are of the type
 * expected at their position.
 */
function ValuesOfCorrectTypeRule(context) {
  return {
    ListValue: function ListValue(node) {
      // Note: TypeInfo will traverse into a list's item type, so look to the
      // parent input type to check if it is a list.
      var type = (0, _definition.getNullableType)(context.getParentInputType());

      if (!(0, _definition.isListType)(type)) {
        isValidValueNode(context, node);
        return false; // Don't traverse further.
      }
    },
    ObjectValue: function ObjectValue(node) {
      var type = (0, _definition.getNamedType)(context.getInputType());

      if (!(0, _definition.isInputObjectType)(type)) {
        isValidValueNode(context, node);
        return false; // Don't traverse further.
      } // Ensure every required field exists.


      var fieldNodeMap = (0, _keyMap.default)(node.fields, function (field) {
        return field.name.value;
      });

      for (var _i2 = 0, _objectValues2 = (0, _objectValues3.default)(type.getFields()); _i2 < _objectValues2.length; _i2++) {
        var fieldDef = _objectValues2[_i2];
        var fieldNode = fieldNodeMap[fieldDef.name];

        if (!fieldNode && (0, _definition.isRequiredInputField)(fieldDef)) {
          var typeStr = (0, _inspect.default)(fieldDef.type);
          context.reportError(new _GraphQLError.GraphQLError("Field \"".concat(type.name, ".").concat(fieldDef.name, "\" of required type \"").concat(typeStr, "\" was not provided."), node));
        }
      }
    },
    ObjectField: function ObjectField(node) {
      var parentType = (0, _definition.getNamedType)(context.getParentInputType());
      var fieldType = context.getInputType();

      if (!fieldType && (0, _definition.isInputObjectType)(parentType)) {
        var suggestions = (0, _suggestionList.default)(node.name.value, Object.keys(parentType.getFields()));
        context.reportError(new _GraphQLError.GraphQLError("Field \"".concat(node.name.value, "\" is not defined by type \"").concat(parentType.name, "\".") + (0, _didYouMean.default)(suggestions), node));
      }
    },
    NullValue: function NullValue(node) {
      var type = context.getInputType();

      if ((0, _definition.isNonNullType)(type)) {
        context.reportError(new _GraphQLError.GraphQLError("Expected value of type \"".concat((0, _inspect.default)(type), "\", found ").concat((0, _printer.print)(node), "."), node));
      }
    },
    EnumValue: function EnumValue(node) {
      return isValidValueNode(context, node);
    },
    IntValue: function IntValue(node) {
      return isValidValueNode(context, node);
    },
    FloatValue: function FloatValue(node) {
      return isValidValueNode(context, node);
    },
    StringValue: function StringValue(node) {
      return isValidValueNode(context, node);
    },
    BooleanValue: function BooleanValue(node) {
      return isValidValueNode(context, node);
    }
  };
}
/**
 * Any value literal may be a valid representation of a Scalar, depending on
 * that scalar type.
 */


function isValidValueNode(context, node) {
  // Report any error at the full type expected by the location.
  var locationType = context.getInputType();

  if (!locationType) {
    return;
  }

  var type = (0, _definition.getNamedType)(locationType);

  if (!(0, _definition.isLeafType)(type)) {
    var typeStr = (0, _inspect.default)(locationType);
    context.reportError(new _GraphQLError.GraphQLError("Expected value of type \"".concat(typeStr, "\", found ").concat((0, _printer.print)(node), "."), node));
    return;
  } // Scalars and Enums determine if a literal value is valid via parseLiteral(),
  // which may throw or return an invalid value to indicate failure.


  try {
    var parseResult = type.parseLiteral(node, undefined
    /* variables */
    );

    if (parseResult === undefined) {
      var _typeStr = (0, _inspect.default)(locationType);

      context.reportError(new _GraphQLError.GraphQLError("Expected value of type \"".concat(_typeStr, "\", found ").concat((0, _printer.print)(node), "."), node));
    }
  } catch (error) {
    var _typeStr2 = (0, _inspect.default)(locationType);

    if (error instanceof _GraphQLError.GraphQLError) {
      context.reportError(error);
    } else {
      context.reportError(new _GraphQLError.GraphQLError("Expected value of type \"".concat(_typeStr2, "\", found ").concat((0, _printer.print)(node), "; ") + error.message, node, undefined, undefined, undefined, error));
    }
  }
}
