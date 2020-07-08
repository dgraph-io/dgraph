"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.astFromValue = astFromValue;

var _isFinite = _interopRequireDefault(require("../polyfills/isFinite"));

var _arrayFrom3 = _interopRequireDefault(require("../polyfills/arrayFrom"));

var _objectValues3 = _interopRequireDefault(require("../polyfills/objectValues"));

var _inspect = _interopRequireDefault(require("../jsutils/inspect"));

var _invariant = _interopRequireDefault(require("../jsutils/invariant"));

var _isObjectLike = _interopRequireDefault(require("../jsutils/isObjectLike"));

var _isCollection = _interopRequireDefault(require("../jsutils/isCollection"));

var _kinds = require("../language/kinds");

var _scalars = require("../type/scalars");

var _definition = require("../type/definition");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Produces a GraphQL Value AST given a JavaScript object.
 * Function will match JavaScript/JSON values to GraphQL AST schema format
 * by using suggested GraphQLInputType. For example:
 *
 *     astFromValue("value", GraphQLString)
 *
 * A GraphQL type must be provided, which will be used to interpret different
 * JavaScript values.
 *
 * | JSON Value    | GraphQL Value        |
 * | ------------- | -------------------- |
 * | Object        | Input Object         |
 * | Array         | List                 |
 * | Boolean       | Boolean              |
 * | String        | String / Enum Value  |
 * | Number        | Int / Float          |
 * | Mixed         | Enum Value           |
 * | null          | NullValue            |
 *
 */
function astFromValue(value, type) {
  if ((0, _definition.isNonNullType)(type)) {
    var astValue = astFromValue(value, type.ofType);

    if ((astValue === null || astValue === void 0 ? void 0 : astValue.kind) === _kinds.Kind.NULL) {
      return null;
    }

    return astValue;
  } // only explicit null, not undefined, NaN


  if (value === null) {
    return {
      kind: _kinds.Kind.NULL
    };
  } // undefined


  if (value === undefined) {
    return null;
  } // Convert JavaScript array to GraphQL list. If the GraphQLType is a list, but
  // the value is not an array, convert the value using the list's item type.


  if ((0, _definition.isListType)(type)) {
    var itemType = type.ofType;

    if ((0, _isCollection.default)(value)) {
      var valuesNodes = []; // Since we transpile for-of in loose mode it doesn't support iterators
      // and it's required to first convert iteratable into array

      for (var _i2 = 0, _arrayFrom2 = (0, _arrayFrom3.default)(value); _i2 < _arrayFrom2.length; _i2++) {
        var item = _arrayFrom2[_i2];
        var itemNode = astFromValue(item, itemType);

        if (itemNode != null) {
          valuesNodes.push(itemNode);
        }
      }

      return {
        kind: _kinds.Kind.LIST,
        values: valuesNodes
      };
    }

    return astFromValue(value, itemType);
  } // Populate the fields of the input object by creating ASTs from each value
  // in the JavaScript object according to the fields in the input type.


  if ((0, _definition.isInputObjectType)(type)) {
    if (!(0, _isObjectLike.default)(value)) {
      return null;
    }

    var fieldNodes = [];

    for (var _i4 = 0, _objectValues2 = (0, _objectValues3.default)(type.getFields()); _i4 < _objectValues2.length; _i4++) {
      var field = _objectValues2[_i4];
      var fieldValue = astFromValue(value[field.name], field.type);

      if (fieldValue) {
        fieldNodes.push({
          kind: _kinds.Kind.OBJECT_FIELD,
          name: {
            kind: _kinds.Kind.NAME,
            value: field.name
          },
          value: fieldValue
        });
      }
    }

    return {
      kind: _kinds.Kind.OBJECT,
      fields: fieldNodes
    };
  } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


  if ((0, _definition.isLeafType)(type)) {
    // Since value is an internally represented value, it must be serialized
    // to an externally represented value before converting into an AST.
    var serialized = type.serialize(value);

    if (serialized == null) {
      return null;
    } // Others serialize based on their corresponding JavaScript scalar types.


    if (typeof serialized === 'boolean') {
      return {
        kind: _kinds.Kind.BOOLEAN,
        value: serialized
      };
    } // JavaScript numbers can be Int or Float values.


    if (typeof serialized === 'number' && (0, _isFinite.default)(serialized)) {
      var stringNum = String(serialized);
      return integerStringRegExp.test(stringNum) ? {
        kind: _kinds.Kind.INT,
        value: stringNum
      } : {
        kind: _kinds.Kind.FLOAT,
        value: stringNum
      };
    }

    if (typeof serialized === 'string') {
      // Enum types use Enum literals.
      if ((0, _definition.isEnumType)(type)) {
        return {
          kind: _kinds.Kind.ENUM,
          value: serialized
        };
      } // ID types can use Int literals.


      if (type === _scalars.GraphQLID && integerStringRegExp.test(serialized)) {
        return {
          kind: _kinds.Kind.INT,
          value: serialized
        };
      }

      return {
        kind: _kinds.Kind.STRING,
        value: serialized
      };
    }

    throw new TypeError("Cannot convert value to AST: ".concat((0, _inspect.default)(serialized), "."));
  } // istanbul ignore next (Not reachable. All possible input types have been considered)


  false || (0, _invariant.default)(0, 'Unexpected input type: ' + (0, _inspect.default)(type));
}
/**
 * IntValue:
 *   - NegativeSign? 0
 *   - NegativeSign? NonZeroDigit ( Digit+ )?
 */


var integerStringRegExp = /^-?(?:0|[1-9][0-9]*)$/;
