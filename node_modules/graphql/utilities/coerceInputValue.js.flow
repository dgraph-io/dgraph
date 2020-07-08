// @flow strict

import arrayFrom from '../polyfills/arrayFrom';
import objectValues from '../polyfills/objectValues';

import type { Path } from '../jsutils/Path';
import inspect from '../jsutils/inspect';
import invariant from '../jsutils/invariant';
import didYouMean from '../jsutils/didYouMean';
import isObjectLike from '../jsutils/isObjectLike';
import isCollection from '../jsutils/isCollection';
import suggestionList from '../jsutils/suggestionList';
import printPathArray from '../jsutils/printPathArray';
import { addPath, pathToArray } from '../jsutils/Path';

import { GraphQLError } from '../error/GraphQLError';

import type { GraphQLInputType } from '../type/definition';
import {
  isLeafType,
  isInputObjectType,
  isListType,
  isNonNullType,
} from '../type/definition';

type OnErrorCB = (
  path: $ReadOnlyArray<string | number>,
  invalidValue: mixed,
  error: GraphQLError,
) => void;

/**
 * Coerces a JavaScript value given a GraphQL Input Type.
 */
export function coerceInputValue(
  inputValue: mixed,
  type: GraphQLInputType,
  onError?: OnErrorCB = defaultOnError,
): mixed {
  return coerceInputValueImpl(inputValue, type, onError);
}

function defaultOnError(
  path: $ReadOnlyArray<string | number>,
  invalidValue: mixed,
  error: GraphQLError,
) {
  let errorPrefix = 'Invalid value ' + inspect(invalidValue);
  if (path.length > 0) {
    errorPrefix += ` at "value${printPathArray(path)}"`;
  }
  error.message = errorPrefix + ': ' + error.message;
  throw error;
}

function coerceInputValueImpl(
  inputValue: mixed,
  type: GraphQLInputType,
  onError: OnErrorCB,
  path: Path | void,
): mixed {
  if (isNonNullType(type)) {
    if (inputValue != null) {
      return coerceInputValueImpl(inputValue, type.ofType, onError, path);
    }
    onError(
      pathToArray(path),
      inputValue,
      new GraphQLError(
        `Expected non-nullable type "${inspect(type)}" not to be null.`,
      ),
    );
    return;
  }

  if (inputValue == null) {
    // Explicitly return the value null.
    return null;
  }

  if (isListType(type)) {
    const itemType = type.ofType;
    if (isCollection(inputValue)) {
      return arrayFrom(inputValue, (itemValue, index) => {
        const itemPath = addPath(path, index, undefined);
        return coerceInputValueImpl(itemValue, itemType, onError, itemPath);
      });
    }
    // Lists accept a non-list value as a list of one.
    return [coerceInputValueImpl(inputValue, itemType, onError, path)];
  }

  if (isInputObjectType(type)) {
    if (!isObjectLike(inputValue)) {
      onError(
        pathToArray(path),
        inputValue,
        new GraphQLError(`Expected type "${type.name}" to be an object.`),
      );
      return;
    }

    const coercedValue = {};
    const fieldDefs = type.getFields();

    for (const field of objectValues(fieldDefs)) {
      const fieldValue = inputValue[field.name];

      if (fieldValue === undefined) {
        if (field.defaultValue !== undefined) {
          coercedValue[field.name] = field.defaultValue;
        } else if (isNonNullType(field.type)) {
          const typeStr = inspect(field.type);
          onError(
            pathToArray(path),
            inputValue,
            new GraphQLError(
              `Field "${field.name}" of required type "${typeStr}" was not provided.`,
            ),
          );
        }
        continue;
      }

      coercedValue[field.name] = coerceInputValueImpl(
        fieldValue,
        field.type,
        onError,
        addPath(path, field.name, type.name),
      );
    }

    // Ensure every provided field is defined.
    for (const fieldName of Object.keys(inputValue)) {
      if (!fieldDefs[fieldName]) {
        const suggestions = suggestionList(
          fieldName,
          Object.keys(type.getFields()),
        );
        onError(
          pathToArray(path),
          inputValue,
          new GraphQLError(
            `Field "${fieldName}" is not defined by type "${type.name}".` +
              didYouMean(suggestions),
          ),
        );
      }
    }
    return coercedValue;
  }

  // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')
  if (isLeafType(type)) {
    let parseResult;

    // Scalars and Enums determine if a input value is valid via parseValue(),
    // which can throw to indicate failure. If it throws, maintain a reference
    // to the original error.
    try {
      parseResult = type.parseValue(inputValue);
    } catch (error) {
      if (error instanceof GraphQLError) {
        onError(pathToArray(path), inputValue, error);
      } else {
        onError(
          pathToArray(path),
          inputValue,
          new GraphQLError(
            `Expected type "${type.name}". ` + error.message,
            undefined,
            undefined,
            undefined,
            undefined,
            error,
          ),
        );
      }
      return;
    }
    if (parseResult === undefined) {
      onError(
        pathToArray(path),
        inputValue,
        new GraphQLError(`Expected type "${type.name}".`),
      );
    }
    return parseResult;
  }

  // istanbul ignore next (Not reachable. All possible input types have been considered)
  invariant(false, 'Unexpected input type: ' + inspect((type: empty)));
}
