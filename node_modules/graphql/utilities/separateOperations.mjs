import { Kind } from "../language/kinds.mjs";
import { visit } from "../language/visitor.mjs";
/**
 * separateOperations accepts a single AST document which may contain many
 * operations and fragments and returns a collection of AST documents each of
 * which contains a single operation as well the fragment definitions it
 * refers to.
 */

export function separateOperations(documentAST) {
  var operations = [];
  var depGraph = Object.create(null);
  var fromName; // Populate metadata and build a dependency graph.

  visit(documentAST, {
    OperationDefinition: function OperationDefinition(node) {
      fromName = opName(node);
      operations.push(node);
    },
    FragmentDefinition: function FragmentDefinition(node) {
      fromName = node.name.value;
    },
    FragmentSpread: function FragmentSpread(node) {
      var toName = node.name.value;
      var dependents = depGraph[fromName];

      if (dependents === undefined) {
        dependents = depGraph[fromName] = Object.create(null);
      }

      dependents[toName] = true;
    }
  }); // For each operation, produce a new synthesized AST which includes only what
  // is necessary for completing that operation.

  var separatedDocumentASTs = Object.create(null);

  var _loop = function _loop(_i2) {
    var operation = operations[_i2];
    var operationName = opName(operation);
    var dependencies = Object.create(null);
    collectTransitiveDependencies(dependencies, depGraph, operationName); // The list of definition nodes to be included for this operation, sorted
    // to retain the same order as the original document.

    separatedDocumentASTs[operationName] = {
      kind: Kind.DOCUMENT,
      definitions: documentAST.definitions.filter(function (node) {
        return node === operation || node.kind === Kind.FRAGMENT_DEFINITION && dependencies[node.name.value];
      })
    };
  };

  for (var _i2 = 0; _i2 < operations.length; _i2++) {
    _loop(_i2);
  }

  return separatedDocumentASTs;
}

// Provides the empty string for anonymous operations.
function opName(operation) {
  return operation.name ? operation.name.value : '';
} // From a dependency graph, collects a list of transitive dependencies by
// recursing through a dependency graph.


function collectTransitiveDependencies(collected, depGraph, fromName) {
  var immediateDeps = depGraph[fromName];

  if (immediateDeps) {
    for (var _i4 = 0, _Object$keys2 = Object.keys(immediateDeps); _i4 < _Object$keys2.length; _i4++) {
      var toName = _Object$keys2[_i4];

      if (!collected[toName]) {
        collected[toName] = true;
        collectTransitiveDependencies(collected, depGraph, toName);
      }
    }
  }
}
