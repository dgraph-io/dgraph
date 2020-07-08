"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.ExecutableDefinitionsRule = ExecutableDefinitionsRule;

var _GraphQLError = require("../../error/GraphQLError");

var _kinds = require("../../language/kinds");

var _predicates = require("../../language/predicates");

/**
 * Executable definitions
 *
 * A GraphQL document is only valid for execution if all definitions are either
 * operation or fragment definitions.
 */
function ExecutableDefinitionsRule(context) {
  return {
    Document: function Document(node) {
      for (var _i2 = 0, _node$definitions2 = node.definitions; _i2 < _node$definitions2.length; _i2++) {
        var definition = _node$definitions2[_i2];

        if (!(0, _predicates.isExecutableDefinitionNode)(definition)) {
          var defName = definition.kind === _kinds.Kind.SCHEMA_DEFINITION || definition.kind === _kinds.Kind.SCHEMA_EXTENSION ? 'schema' : '"' + definition.name.value + '"';
          context.reportError(new _GraphQLError.GraphQLError("The ".concat(defName, " definition is not executable."), definition));
        }
      }

      return false;
    }
  };
}
