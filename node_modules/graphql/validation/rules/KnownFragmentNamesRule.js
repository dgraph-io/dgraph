"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.KnownFragmentNamesRule = KnownFragmentNamesRule;

var _GraphQLError = require("../../error/GraphQLError");

/**
 * Known fragment names
 *
 * A GraphQL document is only valid if all `...Fragment` fragment spreads refer
 * to fragments defined in the same document.
 */
function KnownFragmentNamesRule(context) {
  return {
    FragmentSpread: function FragmentSpread(node) {
      var fragmentName = node.name.value;
      var fragment = context.getFragment(fragmentName);

      if (!fragment) {
        context.reportError(new _GraphQLError.GraphQLError("Unknown fragment \"".concat(fragmentName, "\"."), node.name));
      }
    }
  };
}
