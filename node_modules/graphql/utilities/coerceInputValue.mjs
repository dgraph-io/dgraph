import arrayFrom from "../polyfills/arrayFrom.mjs";
import objectValues from "../polyfills/objectValues.mjs";
import inspect from "../jsutils/inspect.mjs";
import invariant from "../jsutils/invariant.mjs";
import didYouMean from "../jsutils/didYouMean.mjs";
import isObjectLike from "../jsutils/isObjectLike.mjs";
import isCollection from "../jsutils/isCollection.mjs";
import suggestionList from "../jsutils/suggestionList.mjs";
import printPathArray from "../jsutils/printPathArray.mjs";
import { addPath, pathToArray } from "../jsutils/Path.mjs";
import { GraphQLError } from "../error/GraphQLError.mjs";
import { isLeafType, isInputObjectType, isListType, isNonNullType } from "../type/definition.mjs";

/**
 * Coerces a JavaScript value given a GraphQL Input Type.
 */
export function coerceInputValue(inputValue, type) {
  var onError = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : defaultOnError;
  return coerceInputValueImpl(inputValue, type, onError);
}

function defaultOnError(path, invalidValue, error) {
  var errorPrefix = 'Invalid value ' + inspect(invalidValue);

  if (path.length > 0) {
    errorPrefix += " at \"value".concat(printPathArray(path), "\"");
  }

  error.message = errorPrefix + ': ' + error.message;
  throw error;
}

function coerceInputValueImpl(inputValue, type, onError, path) {
  if (isNonNullType(type)) {
    if (inputValue != null) {
      return coerceInputValueImpl(inputValue, type.ofType, onError, path);
    }

    onError(pathToArray(path), inputValue, new GraphQLError("Expected non-nullable type \"".concat(inspect(type), "\" not to be null.")));
    return;
  }

  if (inputValue == null) {
    // Explicitly return the value null.
    return null;
  }

  if (isListType(type)) {
    var itemType = type.ofType;

    if (isCollection(inputValue)) {
      return arrayFrom(inputValue, function (itemValue, index) {
        var itemPath = addPath(path, index, undefined);
        return coerceInputValueImpl(itemValue, itemType, onError, itemPath);
      });
    } // Lists accept a non-list value as a list of one.


    return [coerceInputValueImpl(inputValue, itemType, onError, path)];
  }

  if (isInputObjectType(type)) {
    if (!isObjectLike(inputValue)) {
      onError(pathToArray(path), inputValue, new GraphQLError("Expected type \"".concat(type.name, "\" to be an object.")));
      return;
    }

    var coercedValue = {};
    var fieldDefs = type.getFields();

    for (var _i2 = 0, _objectValues2 = objectValues(fieldDefs); _i2 < _objectValues2.length; _i2++) {
      var field = _objectValues2[_i2];
      var fieldValue = inputValue[field.name];

      if (fieldValue === undefined) {
        if (field.defaultValue !== undefined) {
          coercedValue[field.name] = field.defaultValue;
        } else if (isNonNullType(field.type)) {
          var typeStr = inspect(field.type);
          onError(pathToArray(path), inputValue, new GraphQLError("Field \"".concat(field.name, "\" of required type \"").concat(typeStr, "\" was not provided.")));
        }

        continue;
      }

      coercedValue[field.name] = coerceInputValueImpl(fieldValue, field.type, onError, addPath(path, field.name, type.name));
    } // Ensure every provided field is defined.


    for (var _i4 = 0, _Object$keys2 = Object.keys(inputValue); _i4 < _Object$keys2.length; _i4++) {
      var fieldName = _Object$keys2[_i4];

      if (!fieldDefs[fieldName]) {
        var suggestions = suggestionList(fieldName, Object.keys(type.getFields()));
        onError(pathToArray(path), inputValue, new GraphQLError("Field \"".concat(fieldName, "\" is not defined by type \"").concat(type.name, "\".") + didYouMean(suggestions)));
      }
    }

    return coercedValue;
  } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


  if (isLeafType(type)) {
    var parseResult; // Scalars and Enums determine if a input value is valid via parseValue(),
    // which can throw to indicate failure. If it throws, maintain a reference
    // to the original error.

    try {
      parseResult = type.parseValue(inputValue);
    } catch (error) {
      if (error instanceof GraphQLError) {
        onError(pathToArray(path), inputValue, error);
      } else {
        onError(pathToArray(path), inputValue, new GraphQLError("Expected type \"".concat(type.name, "\". ") + error.message, undefined, undefined, undefined, undefined, error));
      }

      return;
    }

    if (parseResult === undefined) {
      onError(pathToArray(path), inputValue, new GraphQLError("Expected type \"".concat(type.name, "\".")));
    }

    return parseResult;
  } // istanbul ignore next (Not reachable. All possible input types have been considered)


  false || invariant(0, 'Unexpected input type: ' + inspect(type));
}
