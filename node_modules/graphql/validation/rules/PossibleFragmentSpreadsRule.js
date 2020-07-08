"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.PossibleFragmentSpreadsRule = PossibleFragmentSpreadsRule;

var _inspect = _interopRequireDefault(require("../../jsutils/inspect"));

var _GraphQLError = require("../../error/GraphQLError");

var _definition = require("../../type/definition");

var _typeFromAST = require("../../utilities/typeFromAST");

var _typeComparators = require("../../utilities/typeComparators");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Possible fragment spread
 *
 * A fragment spread is only valid if the type condition could ever possibly
 * be true: if there is a non-empty intersection of the possible parent types,
 * and possible types which pass the type condition.
 */
function PossibleFragmentSpreadsRule(context) {
  return {
    InlineFragment: function InlineFragment(node) {
      var fragType = context.getType();
      var parentType = context.getParentType();

      if ((0, _definition.isCompositeType)(fragType) && (0, _definition.isCompositeType)(parentType) && !(0, _typeComparators.doTypesOverlap)(context.getSchema(), fragType, parentType)) {
        var parentTypeStr = (0, _inspect.default)(parentType);
        var fragTypeStr = (0, _inspect.default)(fragType);
        context.reportError(new _GraphQLError.GraphQLError("Fragment cannot be spread here as objects of type \"".concat(parentTypeStr, "\" can never be of type \"").concat(fragTypeStr, "\"."), node));
      }
    },
    FragmentSpread: function FragmentSpread(node) {
      var fragName = node.name.value;
      var fragType = getFragmentType(context, fragName);
      var parentType = context.getParentType();

      if (fragType && parentType && !(0, _typeComparators.doTypesOverlap)(context.getSchema(), fragType, parentType)) {
        var parentTypeStr = (0, _inspect.default)(parentType);
        var fragTypeStr = (0, _inspect.default)(fragType);
        context.reportError(new _GraphQLError.GraphQLError("Fragment \"".concat(fragName, "\" cannot be spread here as objects of type \"").concat(parentTypeStr, "\" can never be of type \"").concat(fragTypeStr, "\"."), node));
      }
    }
  };
}

function getFragmentType(context, name) {
  var frag = context.getFragment(name);

  if (frag) {
    var type = (0, _typeFromAST.typeFromAST)(context.getSchema(), frag.typeCondition);

    if ((0, _definition.isCompositeType)(type)) {
      return type;
    }
  }
}
