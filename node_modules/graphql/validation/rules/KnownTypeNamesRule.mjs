import didYouMean from "../../jsutils/didYouMean.mjs";
import suggestionList from "../../jsutils/suggestionList.mjs";
import { GraphQLError } from "../../error/GraphQLError.mjs";
import { isTypeDefinitionNode, isTypeSystemDefinitionNode, isTypeSystemExtensionNode } from "../../language/predicates.mjs";
import { specifiedScalarTypes } from "../../type/scalars.mjs";
import { introspectionTypes } from "../../type/introspection.mjs";

/**
 * Known type names
 *
 * A GraphQL document is only valid if referenced types (specifically
 * variable definitions and fragment conditions) are defined by the type schema.
 */
export function KnownTypeNamesRule(context) {
  var schema = context.getSchema();
  var existingTypesMap = schema ? schema.getTypeMap() : Object.create(null);
  var definedTypes = Object.create(null);

  for (var _i2 = 0, _context$getDocument$2 = context.getDocument().definitions; _i2 < _context$getDocument$2.length; _i2++) {
    var def = _context$getDocument$2[_i2];

    if (isTypeDefinitionNode(def)) {
      definedTypes[def.name.value] = true;
    }
  }

  var typeNames = Object.keys(existingTypesMap).concat(Object.keys(definedTypes));
  return {
    NamedType: function NamedType(node, _1, parent, _2, ancestors) {
      var typeName = node.name.value;

      if (!existingTypesMap[typeName] && !definedTypes[typeName]) {
        var _ancestors$;

        var definitionNode = (_ancestors$ = ancestors[2]) !== null && _ancestors$ !== void 0 ? _ancestors$ : parent;
        var isSDL = definitionNode != null && isSDLNode(definitionNode);

        if (isSDL && isStandardTypeName(typeName)) {
          return;
        }

        var suggestedTypes = suggestionList(typeName, isSDL ? standardTypeNames.concat(typeNames) : typeNames);
        context.reportError(new GraphQLError("Unknown type \"".concat(typeName, "\".") + didYouMean(suggestedTypes), node));
      }
    }
  };
}
var standardTypeNames = [].concat(specifiedScalarTypes, introspectionTypes).map(function (type) {
  return type.name;
});

function isStandardTypeName(typeName) {
  return standardTypeNames.indexOf(typeName) !== -1;
}

function isSDLNode(value) {
  return !Array.isArray(value) && (isTypeSystemDefinitionNode(value) || isTypeSystemExtensionNode(value));
}
