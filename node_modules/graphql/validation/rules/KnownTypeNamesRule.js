"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.KnownTypeNamesRule = KnownTypeNamesRule;

var _didYouMean = _interopRequireDefault(require("../../jsutils/didYouMean"));

var _suggestionList = _interopRequireDefault(require("../../jsutils/suggestionList"));

var _GraphQLError = require("../../error/GraphQLError");

var _predicates = require("../../language/predicates");

var _scalars = require("../../type/scalars");

var _introspection = require("../../type/introspection");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Known type names
 *
 * A GraphQL document is only valid if referenced types (specifically
 * variable definitions and fragment conditions) are defined by the type schema.
 */
function KnownTypeNamesRule(context) {
  var schema = context.getSchema();
  var existingTypesMap = schema ? schema.getTypeMap() : Object.create(null);
  var definedTypes = Object.create(null);

  for (var _i2 = 0, _context$getDocument$2 = context.getDocument().definitions; _i2 < _context$getDocument$2.length; _i2++) {
    var def = _context$getDocument$2[_i2];

    if ((0, _predicates.isTypeDefinitionNode)(def)) {
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

        var suggestedTypes = (0, _suggestionList.default)(typeName, isSDL ? standardTypeNames.concat(typeNames) : typeNames);
        context.reportError(new _GraphQLError.GraphQLError("Unknown type \"".concat(typeName, "\".") + (0, _didYouMean.default)(suggestedTypes), node));
      }
    }
  };
}

var standardTypeNames = [].concat(_scalars.specifiedScalarTypes, _introspection.introspectionTypes).map(function (type) {
  return type.name;
});

function isStandardTypeName(typeName) {
  return standardTypeNames.indexOf(typeName) !== -1;
}

function isSDLNode(value) {
  return !Array.isArray(value) && ((0, _predicates.isTypeSystemDefinitionNode)(value) || (0, _predicates.isTypeSystemExtensionNode)(value));
}
