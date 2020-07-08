import isFinite from "../polyfills/isFinite.mjs";
import arrayFrom from "../polyfills/arrayFrom.mjs";
import objectValues from "../polyfills/objectValues.mjs";
import inspect from "../jsutils/inspect.mjs";
import invariant from "../jsutils/invariant.mjs";
import isObjectLike from "../jsutils/isObjectLike.mjs";
import isCollection from "../jsutils/isCollection.mjs";
import { Kind } from "../language/kinds.mjs";
import { GraphQLID } from "../type/scalars.mjs";
import { isLeafType, isEnumType, isInputObjectType, isListType, isNonNullType } from "../type/definition.mjs";
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

export function astFromValue(value, type) {
  if (isNonNullType(type)) {
    var astValue = astFromValue(value, type.ofType);

    if ((astValue === null || astValue === void 0 ? void 0 : astValue.kind) === Kind.NULL) {
      return null;
    }

    return astValue;
  } // only explicit null, not undefined, NaN


  if (value === null) {
    return {
      kind: Kind.NULL
    };
  } // undefined


  if (value === undefined) {
    return null;
  } // Convert JavaScript array to GraphQL list. If the GraphQLType is a list, but
  // the value is not an array, convert the value using the list's item type.


  if (isListType(type)) {
    var itemType = type.ofType;

    if (isCollection(value)) {
      var valuesNodes = []; // Since we transpile for-of in loose mode it doesn't support iterators
      // and it's required to first convert iteratable into array

      for (var _i2 = 0, _arrayFrom2 = arrayFrom(value); _i2 < _arrayFrom2.length; _i2++) {
        var item = _arrayFrom2[_i2];
        var itemNode = astFromValue(item, itemType);

        if (itemNode != null) {
          valuesNodes.push(itemNode);
        }
      }

      return {
        kind: Kind.LIST,
        values: valuesNodes
      };
    }

    return astFromValue(value, itemType);
  } // Populate the fields of the input object by creating ASTs from each value
  // in the JavaScript object according to the fields in the input type.


  if (isInputObjectType(type)) {
    if (!isObjectLike(value)) {
      return null;
    }

    var fieldNodes = [];

    for (var _i4 = 0, _objectValues2 = objectValues(type.getFields()); _i4 < _objectValues2.length; _i4++) {
      var field = _objectValues2[_i4];
      var fieldValue = astFromValue(value[field.name], field.type);

      if (fieldValue) {
        fieldNodes.push({
          kind: Kind.OBJECT_FIELD,
          name: {
            kind: Kind.NAME,
            value: field.name
          },
          value: fieldValue
        });
      }
    }

    return {
      kind: Kind.OBJECT,
      fields: fieldNodes
    };
  } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


  if (isLeafType(type)) {
    // Since value is an internally represented value, it must be serialized
    // to an externally represented value before converting into an AST.
    var serialized = type.serialize(value);

    if (serialized == null) {
      return null;
    } // Others serialize based on their corresponding JavaScript scalar types.


    if (typeof serialized === 'boolean') {
      return {
        kind: Kind.BOOLEAN,
        value: serialized
      };
    } // JavaScript numbers can be Int or Float values.


    if (typeof serialized === 'number' && isFinite(serialized)) {
      var stringNum = String(serialized);
      return integerStringRegExp.test(stringNum) ? {
        kind: Kind.INT,
        value: stringNum
      } : {
        kind: Kind.FLOAT,
        value: stringNum
      };
    }

    if (typeof serialized === 'string') {
      // Enum types use Enum literals.
      if (isEnumType(type)) {
        return {
          kind: Kind.ENUM,
          value: serialized
        };
      } // ID types can use Int literals.


      if (type === GraphQLID && integerStringRegExp.test(serialized)) {
        return {
          kind: Kind.INT,
          value: serialized
        };
      }

      return {
        kind: Kind.STRING,
        value: serialized
      };
    }

    throw new TypeError("Cannot convert value to AST: ".concat(inspect(serialized), "."));
  } // istanbul ignore next (Not reachable. All possible input types have been considered)


  false || invariant(0, 'Unexpected input type: ' + inspect(type));
}
/**
 * IntValue:
 *   - NegativeSign? 0
 *   - NegativeSign? NonZeroDigit ( Digit+ )?
 */

var integerStringRegExp = /^-?(?:0|[1-9][0-9]*)$/;
