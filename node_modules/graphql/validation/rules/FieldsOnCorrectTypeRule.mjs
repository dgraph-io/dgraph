import arrayFrom from "../../polyfills/arrayFrom.mjs";
import didYouMean from "../../jsutils/didYouMean.mjs";
import suggestionList from "../../jsutils/suggestionList.mjs";
import { GraphQLError } from "../../error/GraphQLError.mjs";
import { isObjectType, isInterfaceType, isAbstractType } from "../../type/definition.mjs";

/**
 * Fields on correct type
 *
 * A GraphQL document is only valid if all fields selected are defined by the
 * parent type, or are an allowed meta field such as __typename.
 */
export function FieldsOnCorrectTypeRule(context) {
  return {
    Field: function Field(node) {
      var type = context.getParentType();

      if (type) {
        var fieldDef = context.getFieldDef();

        if (!fieldDef) {
          // This field doesn't exist, lets look for suggestions.
          var schema = context.getSchema();
          var fieldName = node.name.value; // First determine if there are any suggested types to condition on.

          var suggestion = didYouMean('to use an inline fragment on', getSuggestedTypeNames(schema, type, fieldName)); // If there are no suggested types, then perhaps this was a typo?

          if (suggestion === '') {
            suggestion = didYouMean(getSuggestedFieldNames(type, fieldName));
          } // Report an error, including helpful suggestions.


          context.reportError(new GraphQLError("Cannot query field \"".concat(fieldName, "\" on type \"").concat(type.name, "\".") + suggestion, node));
        }
      }
    }
  };
}
/**
 * Go through all of the implementations of type, as well as the interfaces that
 * they implement. If any of those types include the provided field, suggest them,
 * sorted by how often the type is referenced.
 */

function getSuggestedTypeNames(schema, type, fieldName) {
  if (!isAbstractType(type)) {
    // Must be an Object type, which does not have possible fields.
    return [];
  }

  var suggestedTypes = new Set();
  var usageCount = Object.create(null);

  for (var _i2 = 0, _schema$getPossibleTy2 = schema.getPossibleTypes(type); _i2 < _schema$getPossibleTy2.length; _i2++) {
    var possibleType = _schema$getPossibleTy2[_i2];

    if (!possibleType.getFields()[fieldName]) {
      continue;
    } // This object type defines this field.


    suggestedTypes.add(possibleType);
    usageCount[possibleType.name] = 1;

    for (var _i4 = 0, _possibleType$getInte2 = possibleType.getInterfaces(); _i4 < _possibleType$getInte2.length; _i4++) {
      var _usageCount$possibleI;

      var possibleInterface = _possibleType$getInte2[_i4];

      if (!possibleInterface.getFields()[fieldName]) {
        continue;
      } // This interface type defines this field.


      suggestedTypes.add(possibleInterface);
      usageCount[possibleInterface.name] = ((_usageCount$possibleI = usageCount[possibleInterface.name]) !== null && _usageCount$possibleI !== void 0 ? _usageCount$possibleI : 0) + 1;
    }
  }

  return arrayFrom(suggestedTypes).sort(function (typeA, typeB) {
    // Suggest both interface and object types based on how common they are.
    var usageCountDiff = usageCount[typeB.name] - usageCount[typeA.name];

    if (usageCountDiff !== 0) {
      return usageCountDiff;
    } // Suggest super types first followed by subtypes


    if (isInterfaceType(typeA) && schema.isSubType(typeA, typeB)) {
      return -1;
    }

    if (isInterfaceType(typeB) && schema.isSubType(typeB, typeA)) {
      return 1;
    }

    return typeA.name.localeCompare(typeB.name);
  }).map(function (x) {
    return x.name;
  });
}
/**
 * For the field name provided, determine if there are any similar field names
 * that may be the result of a typo.
 */


function getSuggestedFieldNames(type, fieldName) {
  if (isObjectType(type) || isInterfaceType(type)) {
    var possibleFieldNames = Object.keys(type.getFields());
    return suggestionList(fieldName, possibleFieldNames);
  } // Otherwise, must be a Union type, which does not define fields.


  return [];
}
