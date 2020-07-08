"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var ts_invariant_1 = require("ts-invariant");
var assign_1 = require("./util/assign");
var storeUtils_1 = require("./storeUtils");
function getMutationDefinition(doc) {
    checkDocument(doc);
    var mutationDef = doc.definitions.filter(function (definition) {
        return definition.kind === 'OperationDefinition' &&
            definition.operation === 'mutation';
    })[0];
    ts_invariant_1.invariant(mutationDef, 'Must contain a mutation definition.');
    return mutationDef;
}
exports.getMutationDefinition = getMutationDefinition;
function checkDocument(doc) {
    ts_invariant_1.invariant(doc && doc.kind === 'Document', "Expecting a parsed GraphQL document. Perhaps you need to wrap the query string in a \"gql\" tag? http://docs.apollostack.com/apollo-client/core.html#gql");
    var operations = doc.definitions
        .filter(function (d) { return d.kind !== 'FragmentDefinition'; })
        .map(function (definition) {
        if (definition.kind !== 'OperationDefinition') {
            throw new ts_invariant_1.InvariantError("Schema type definitions not allowed in queries. Found: \"" + definition.kind + "\"");
        }
        return definition;
    });
    ts_invariant_1.invariant(operations.length <= 1, "Ambiguous GraphQL document: contains " + operations.length + " operations");
    return doc;
}
exports.checkDocument = checkDocument;
function getOperationDefinition(doc) {
    checkDocument(doc);
    return doc.definitions.filter(function (definition) { return definition.kind === 'OperationDefinition'; })[0];
}
exports.getOperationDefinition = getOperationDefinition;
function getOperationDefinitionOrDie(document) {
    var def = getOperationDefinition(document);
    ts_invariant_1.invariant(def, "GraphQL document is missing an operation");
    return def;
}
exports.getOperationDefinitionOrDie = getOperationDefinitionOrDie;
function getOperationName(doc) {
    return (doc.definitions
        .filter(function (definition) {
        return definition.kind === 'OperationDefinition' && definition.name;
    })
        .map(function (x) { return x.name.value; })[0] || null);
}
exports.getOperationName = getOperationName;
function getFragmentDefinitions(doc) {
    return doc.definitions.filter(function (definition) { return definition.kind === 'FragmentDefinition'; });
}
exports.getFragmentDefinitions = getFragmentDefinitions;
function getQueryDefinition(doc) {
    var queryDef = getOperationDefinition(doc);
    ts_invariant_1.invariant(queryDef && queryDef.operation === 'query', 'Must contain a query definition.');
    return queryDef;
}
exports.getQueryDefinition = getQueryDefinition;
function getFragmentDefinition(doc) {
    ts_invariant_1.invariant(doc.kind === 'Document', "Expecting a parsed GraphQL document. Perhaps you need to wrap the query string in a \"gql\" tag? http://docs.apollostack.com/apollo-client/core.html#gql");
    ts_invariant_1.invariant(doc.definitions.length <= 1, 'Fragment must have exactly one definition.');
    var fragmentDef = doc.definitions[0];
    ts_invariant_1.invariant(fragmentDef.kind === 'FragmentDefinition', 'Must be a fragment definition.');
    return fragmentDef;
}
exports.getFragmentDefinition = getFragmentDefinition;
function getMainDefinition(queryDoc) {
    checkDocument(queryDoc);
    var fragmentDefinition;
    for (var _i = 0, _a = queryDoc.definitions; _i < _a.length; _i++) {
        var definition = _a[_i];
        if (definition.kind === 'OperationDefinition') {
            var operation = definition.operation;
            if (operation === 'query' ||
                operation === 'mutation' ||
                operation === 'subscription') {
                return definition;
            }
        }
        if (definition.kind === 'FragmentDefinition' && !fragmentDefinition) {
            fragmentDefinition = definition;
        }
    }
    if (fragmentDefinition) {
        return fragmentDefinition;
    }
    throw new ts_invariant_1.InvariantError('Expected a parsed GraphQL query with a query, mutation, subscription, or a fragment.');
}
exports.getMainDefinition = getMainDefinition;
function createFragmentMap(fragments) {
    if (fragments === void 0) { fragments = []; }
    var symTable = {};
    fragments.forEach(function (fragment) {
        symTable[fragment.name.value] = fragment;
    });
    return symTable;
}
exports.createFragmentMap = createFragmentMap;
function getDefaultValues(definition) {
    if (definition &&
        definition.variableDefinitions &&
        definition.variableDefinitions.length) {
        var defaultValues = definition.variableDefinitions
            .filter(function (_a) {
            var defaultValue = _a.defaultValue;
            return defaultValue;
        })
            .map(function (_a) {
            var variable = _a.variable, defaultValue = _a.defaultValue;
            var defaultValueObj = {};
            storeUtils_1.valueToObjectRepresentation(defaultValueObj, variable.name, defaultValue);
            return defaultValueObj;
        });
        return assign_1.assign.apply(void 0, tslib_1.__spreadArrays([{}], defaultValues));
    }
    return {};
}
exports.getDefaultValues = getDefaultValues;
function variablesInOperation(operation) {
    var names = new Set();
    if (operation.variableDefinitions) {
        for (var _i = 0, _a = operation.variableDefinitions; _i < _a.length; _i++) {
            var definition = _a[_i];
            names.add(definition.variable.name.value);
        }
    }
    return names;
}
exports.variablesInOperation = variablesInOperation;
//# sourceMappingURL=getFromAST.js.map