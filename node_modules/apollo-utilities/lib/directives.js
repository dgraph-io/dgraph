"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var visitor_1 = require("graphql/language/visitor");
var ts_invariant_1 = require("ts-invariant");
var storeUtils_1 = require("./storeUtils");
function getDirectiveInfoFromField(field, variables) {
    if (field.directives && field.directives.length) {
        var directiveObj_1 = {};
        field.directives.forEach(function (directive) {
            directiveObj_1[directive.name.value] = storeUtils_1.argumentsObjectFromField(directive, variables);
        });
        return directiveObj_1;
    }
    return null;
}
exports.getDirectiveInfoFromField = getDirectiveInfoFromField;
function shouldInclude(selection, variables) {
    if (variables === void 0) { variables = {}; }
    return getInclusionDirectives(selection.directives).every(function (_a) {
        var directive = _a.directive, ifArgument = _a.ifArgument;
        var evaledValue = false;
        if (ifArgument.value.kind === 'Variable') {
            evaledValue = variables[ifArgument.value.name.value];
            ts_invariant_1.invariant(evaledValue !== void 0, "Invalid variable referenced in @" + directive.name.value + " directive.");
        }
        else {
            evaledValue = ifArgument.value.value;
        }
        return directive.name.value === 'skip' ? !evaledValue : evaledValue;
    });
}
exports.shouldInclude = shouldInclude;
function getDirectiveNames(doc) {
    var names = [];
    visitor_1.visit(doc, {
        Directive: function (node) {
            names.push(node.name.value);
        },
    });
    return names;
}
exports.getDirectiveNames = getDirectiveNames;
function hasDirectives(names, doc) {
    return getDirectiveNames(doc).some(function (name) { return names.indexOf(name) > -1; });
}
exports.hasDirectives = hasDirectives;
function hasClientExports(document) {
    return (document &&
        hasDirectives(['client'], document) &&
        hasDirectives(['export'], document));
}
exports.hasClientExports = hasClientExports;
function isInclusionDirective(_a) {
    var value = _a.name.value;
    return value === 'skip' || value === 'include';
}
function getInclusionDirectives(directives) {
    return directives ? directives.filter(isInclusionDirective).map(function (directive) {
        var directiveArguments = directive.arguments;
        var directiveName = directive.name.value;
        ts_invariant_1.invariant(directiveArguments && directiveArguments.length === 1, "Incorrect number of arguments for the @" + directiveName + " directive.");
        var ifArgument = directiveArguments[0];
        ts_invariant_1.invariant(ifArgument.name && ifArgument.name.value === 'if', "Invalid argument for the @" + directiveName + " directive.");
        var ifValue = ifArgument.value;
        ts_invariant_1.invariant(ifValue &&
            (ifValue.kind === 'Variable' || ifValue.kind === 'BooleanValue'), "Argument for the @" + directiveName + " directive must be a variable or a boolean value.");
        return { directive: directive, ifArgument: ifArgument };
    }) : [];
}
exports.getInclusionDirectives = getInclusionDirectives;
//# sourceMappingURL=directives.js.map