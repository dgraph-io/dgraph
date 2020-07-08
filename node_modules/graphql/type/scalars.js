"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.isSpecifiedScalarType = isSpecifiedScalarType;
exports.specifiedScalarTypes = exports.GraphQLID = exports.GraphQLBoolean = exports.GraphQLString = exports.GraphQLFloat = exports.GraphQLInt = void 0;

var _isFinite = _interopRequireDefault(require("../polyfills/isFinite"));

var _isInteger = _interopRequireDefault(require("../polyfills/isInteger"));

var _inspect = _interopRequireDefault(require("../jsutils/inspect"));

var _isObjectLike = _interopRequireDefault(require("../jsutils/isObjectLike"));

var _kinds = require("../language/kinds");

var _printer = require("../language/printer");

var _GraphQLError = require("../error/GraphQLError");

var _definition = require("./definition");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

// As per the GraphQL Spec, Integers are only treated as valid when a valid
// 32-bit signed integer, providing the broadest support across platforms.
//
// n.b. JavaScript's integers are safe between -(2^53 - 1) and 2^53 - 1 because
// they are internally represented as IEEE 754 doubles.
var MAX_INT = 2147483647;
var MIN_INT = -2147483648;

function serializeInt(outputValue) {
  var coercedValue = serializeObject(outputValue);

  if (typeof coercedValue === 'boolean') {
    return coercedValue ? 1 : 0;
  }

  var num = coercedValue;

  if (typeof coercedValue === 'string' && coercedValue !== '') {
    num = Number(coercedValue);
  }

  if (!(0, _isInteger.default)(num)) {
    throw new _GraphQLError.GraphQLError("Int cannot represent non-integer value: ".concat((0, _inspect.default)(coercedValue)));
  }

  if (num > MAX_INT || num < MIN_INT) {
    throw new _GraphQLError.GraphQLError('Int cannot represent non 32-bit signed integer value: ' + (0, _inspect.default)(coercedValue));
  }

  return num;
}

function coerceInt(inputValue) {
  if (!(0, _isInteger.default)(inputValue)) {
    throw new _GraphQLError.GraphQLError("Int cannot represent non-integer value: ".concat((0, _inspect.default)(inputValue)));
  }

  if (inputValue > MAX_INT || inputValue < MIN_INT) {
    throw new _GraphQLError.GraphQLError("Int cannot represent non 32-bit signed integer value: ".concat(inputValue));
  }

  return inputValue;
}

var GraphQLInt = new _definition.GraphQLScalarType({
  name: 'Int',
  description: 'The `Int` scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1.',
  serialize: serializeInt,
  parseValue: coerceInt,
  parseLiteral: function parseLiteral(valueNode) {
    if (valueNode.kind !== _kinds.Kind.INT) {
      throw new _GraphQLError.GraphQLError("Int cannot represent non-integer value: ".concat((0, _printer.print)(valueNode)), valueNode);
    }

    var num = parseInt(valueNode.value, 10);

    if (num > MAX_INT || num < MIN_INT) {
      throw new _GraphQLError.GraphQLError("Int cannot represent non 32-bit signed integer value: ".concat(valueNode.value), valueNode);
    }

    return num;
  }
});
exports.GraphQLInt = GraphQLInt;

function serializeFloat(outputValue) {
  var coercedValue = serializeObject(outputValue);

  if (typeof coercedValue === 'boolean') {
    return coercedValue ? 1 : 0;
  }

  var num = coercedValue;

  if (typeof coercedValue === 'string' && coercedValue !== '') {
    num = Number(coercedValue);
  }

  if (!(0, _isFinite.default)(num)) {
    throw new _GraphQLError.GraphQLError("Float cannot represent non numeric value: ".concat((0, _inspect.default)(coercedValue)));
  }

  return num;
}

function coerceFloat(inputValue) {
  if (!(0, _isFinite.default)(inputValue)) {
    throw new _GraphQLError.GraphQLError("Float cannot represent non numeric value: ".concat((0, _inspect.default)(inputValue)));
  }

  return inputValue;
}

var GraphQLFloat = new _definition.GraphQLScalarType({
  name: 'Float',
  description: 'The `Float` scalar type represents signed double-precision fractional values as specified by [IEEE 754](https://en.wikipedia.org/wiki/IEEE_floating_point).',
  serialize: serializeFloat,
  parseValue: coerceFloat,
  parseLiteral: function parseLiteral(valueNode) {
    if (valueNode.kind !== _kinds.Kind.FLOAT && valueNode.kind !== _kinds.Kind.INT) {
      throw new _GraphQLError.GraphQLError("Float cannot represent non numeric value: ".concat((0, _printer.print)(valueNode)), valueNode);
    }

    return parseFloat(valueNode.value);
  }
}); // Support serializing objects with custom valueOf() or toJSON() functions -
// a common way to represent a complex value which can be represented as
// a string (ex: MongoDB id objects).

exports.GraphQLFloat = GraphQLFloat;

function serializeObject(outputValue) {
  if ((0, _isObjectLike.default)(outputValue)) {
    if (typeof outputValue.valueOf === 'function') {
      var valueOfResult = outputValue.valueOf();

      if (!(0, _isObjectLike.default)(valueOfResult)) {
        return valueOfResult;
      }
    }

    if (typeof outputValue.toJSON === 'function') {
      // $FlowFixMe(>=0.90.0)
      return outputValue.toJSON();
    }
  }

  return outputValue;
}

function serializeString(outputValue) {
  var coercedValue = serializeObject(outputValue); // Serialize string, boolean and number values to a string, but do not
  // attempt to coerce object, function, symbol, or other types as strings.

  if (typeof coercedValue === 'string') {
    return coercedValue;
  }

  if (typeof coercedValue === 'boolean') {
    return coercedValue ? 'true' : 'false';
  }

  if ((0, _isFinite.default)(coercedValue)) {
    return coercedValue.toString();
  }

  throw new _GraphQLError.GraphQLError("String cannot represent value: ".concat((0, _inspect.default)(outputValue)));
}

function coerceString(inputValue) {
  if (typeof inputValue !== 'string') {
    throw new _GraphQLError.GraphQLError("String cannot represent a non string value: ".concat((0, _inspect.default)(inputValue)));
  }

  return inputValue;
}

var GraphQLString = new _definition.GraphQLScalarType({
  name: 'String',
  description: 'The `String` scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text.',
  serialize: serializeString,
  parseValue: coerceString,
  parseLiteral: function parseLiteral(valueNode) {
    if (valueNode.kind !== _kinds.Kind.STRING) {
      throw new _GraphQLError.GraphQLError("String cannot represent a non string value: ".concat((0, _printer.print)(valueNode)), valueNode);
    }

    return valueNode.value;
  }
});
exports.GraphQLString = GraphQLString;

function serializeBoolean(outputValue) {
  var coercedValue = serializeObject(outputValue);

  if (typeof coercedValue === 'boolean') {
    return coercedValue;
  }

  if ((0, _isFinite.default)(coercedValue)) {
    return coercedValue !== 0;
  }

  throw new _GraphQLError.GraphQLError("Boolean cannot represent a non boolean value: ".concat((0, _inspect.default)(coercedValue)));
}

function coerceBoolean(inputValue) {
  if (typeof inputValue !== 'boolean') {
    throw new _GraphQLError.GraphQLError("Boolean cannot represent a non boolean value: ".concat((0, _inspect.default)(inputValue)));
  }

  return inputValue;
}

var GraphQLBoolean = new _definition.GraphQLScalarType({
  name: 'Boolean',
  description: 'The `Boolean` scalar type represents `true` or `false`.',
  serialize: serializeBoolean,
  parseValue: coerceBoolean,
  parseLiteral: function parseLiteral(valueNode) {
    if (valueNode.kind !== _kinds.Kind.BOOLEAN) {
      throw new _GraphQLError.GraphQLError("Boolean cannot represent a non boolean value: ".concat((0, _printer.print)(valueNode)), valueNode);
    }

    return valueNode.value;
  }
});
exports.GraphQLBoolean = GraphQLBoolean;

function serializeID(outputValue) {
  var coercedValue = serializeObject(outputValue);

  if (typeof coercedValue === 'string') {
    return coercedValue;
  }

  if ((0, _isInteger.default)(coercedValue)) {
    return String(coercedValue);
  }

  throw new _GraphQLError.GraphQLError("ID cannot represent value: ".concat((0, _inspect.default)(outputValue)));
}

function coerceID(inputValue) {
  if (typeof inputValue === 'string') {
    return inputValue;
  }

  if ((0, _isInteger.default)(inputValue)) {
    return inputValue.toString();
  }

  throw new _GraphQLError.GraphQLError("ID cannot represent value: ".concat((0, _inspect.default)(inputValue)));
}

var GraphQLID = new _definition.GraphQLScalarType({
  name: 'ID',
  description: 'The `ID` scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as `"4"`) or integer (such as `4`) input value will be accepted as an ID.',
  serialize: serializeID,
  parseValue: coerceID,
  parseLiteral: function parseLiteral(valueNode) {
    if (valueNode.kind !== _kinds.Kind.STRING && valueNode.kind !== _kinds.Kind.INT) {
      throw new _GraphQLError.GraphQLError('ID cannot represent a non-string and non-integer value: ' + (0, _printer.print)(valueNode), valueNode);
    }

    return valueNode.value;
  }
});
exports.GraphQLID = GraphQLID;
var specifiedScalarTypes = Object.freeze([GraphQLString, GraphQLInt, GraphQLFloat, GraphQLBoolean, GraphQLID]);
exports.specifiedScalarTypes = specifiedScalarTypes;

function isSpecifiedScalarType(type) {
  return specifiedScalarTypes.some(function (_ref) {
    var name = _ref.name;
    return type.name === name;
  });
}
