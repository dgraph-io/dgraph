var _defKindToExtKind;

function _defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

import inspect from "../../jsutils/inspect.mjs";
import invariant from "../../jsutils/invariant.mjs";
import didYouMean from "../../jsutils/didYouMean.mjs";
import suggestionList from "../../jsutils/suggestionList.mjs";
import { GraphQLError } from "../../error/GraphQLError.mjs";
import { Kind } from "../../language/kinds.mjs";
import { isTypeDefinitionNode } from "../../language/predicates.mjs";
import { isScalarType, isObjectType, isInterfaceType, isUnionType, isEnumType, isInputObjectType } from "../../type/definition.mjs";

/**
 * Possible type extension
 *
 * A type extension is only valid if the type is defined and has the same kind.
 */
export function PossibleTypeExtensionsRule(context) {
  var schema = context.getSchema();
  var definedTypes = Object.create(null);

  for (var _i2 = 0, _context$getDocument$2 = context.getDocument().definitions; _i2 < _context$getDocument$2.length; _i2++) {
    var def = _context$getDocument$2[_i2];

    if (isTypeDefinitionNode(def)) {
      definedTypes[def.name.value] = def;
    }
  }

  return {
    ScalarTypeExtension: checkExtension,
    ObjectTypeExtension: checkExtension,
    InterfaceTypeExtension: checkExtension,
    UnionTypeExtension: checkExtension,
    EnumTypeExtension: checkExtension,
    InputObjectTypeExtension: checkExtension
  };

  function checkExtension(node) {
    var typeName = node.name.value;
    var defNode = definedTypes[typeName];
    var existingType = schema === null || schema === void 0 ? void 0 : schema.getType(typeName);
    var expectedKind;

    if (defNode) {
      expectedKind = defKindToExtKind[defNode.kind];
    } else if (existingType) {
      expectedKind = typeToExtKind(existingType);
    }

    if (expectedKind) {
      if (expectedKind !== node.kind) {
        var kindStr = extensionKindToTypeName(node.kind);
        context.reportError(new GraphQLError("Cannot extend non-".concat(kindStr, " type \"").concat(typeName, "\"."), defNode ? [defNode, node] : node));
      }
    } else {
      var allTypeNames = Object.keys(definedTypes);

      if (schema) {
        allTypeNames = allTypeNames.concat(Object.keys(schema.getTypeMap()));
      }

      var suggestedTypes = suggestionList(typeName, allTypeNames);
      context.reportError(new GraphQLError("Cannot extend type \"".concat(typeName, "\" because it is not defined.") + didYouMean(suggestedTypes), node.name));
    }
  }
}
var defKindToExtKind = (_defKindToExtKind = {}, _defineProperty(_defKindToExtKind, Kind.SCALAR_TYPE_DEFINITION, Kind.SCALAR_TYPE_EXTENSION), _defineProperty(_defKindToExtKind, Kind.OBJECT_TYPE_DEFINITION, Kind.OBJECT_TYPE_EXTENSION), _defineProperty(_defKindToExtKind, Kind.INTERFACE_TYPE_DEFINITION, Kind.INTERFACE_TYPE_EXTENSION), _defineProperty(_defKindToExtKind, Kind.UNION_TYPE_DEFINITION, Kind.UNION_TYPE_EXTENSION), _defineProperty(_defKindToExtKind, Kind.ENUM_TYPE_DEFINITION, Kind.ENUM_TYPE_EXTENSION), _defineProperty(_defKindToExtKind, Kind.INPUT_OBJECT_TYPE_DEFINITION, Kind.INPUT_OBJECT_TYPE_EXTENSION), _defKindToExtKind);

function typeToExtKind(type) {
  if (isScalarType(type)) {
    return Kind.SCALAR_TYPE_EXTENSION;
  }

  if (isObjectType(type)) {
    return Kind.OBJECT_TYPE_EXTENSION;
  }

  if (isInterfaceType(type)) {
    return Kind.INTERFACE_TYPE_EXTENSION;
  }

  if (isUnionType(type)) {
    return Kind.UNION_TYPE_EXTENSION;
  }

  if (isEnumType(type)) {
    return Kind.ENUM_TYPE_EXTENSION;
  } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


  if (isInputObjectType(type)) {
    return Kind.INPUT_OBJECT_TYPE_EXTENSION;
  } // istanbul ignore next (Not reachable. All possible types have been considered)


  false || invariant(0, 'Unexpected type: ' + inspect(type));
}

function extensionKindToTypeName(kind) {
  switch (kind) {
    case Kind.SCALAR_TYPE_EXTENSION:
      return 'scalar';

    case Kind.OBJECT_TYPE_EXTENSION:
      return 'object';

    case Kind.INTERFACE_TYPE_EXTENSION:
      return 'interface';

    case Kind.UNION_TYPE_EXTENSION:
      return 'union';

    case Kind.ENUM_TYPE_EXTENSION:
      return 'enum';

    case Kind.INPUT_OBJECT_TYPE_EXTENSION:
      return 'input object';
  } // istanbul ignore next (Not reachable. All possible types have been considered)


  false || invariant(0, 'Unexpected kind: ' + inspect(kind));
}
