"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.KnownArgumentNamesRule = KnownArgumentNamesRule;
exports.KnownArgumentNamesOnDirectivesRule = KnownArgumentNamesOnDirectivesRule;

var _didYouMean = _interopRequireDefault(require("../../jsutils/didYouMean"));

var _suggestionList = _interopRequireDefault(require("../../jsutils/suggestionList"));

var _GraphQLError = require("../../error/GraphQLError");

var _kinds = require("../../language/kinds");

var _directives = require("../../type/directives");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

function ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); keys.push.apply(keys, symbols); } return keys; }

function _objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { ownKeys(Object(source), true).forEach(function (key) { _defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function _defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

/**
 * Known argument names
 *
 * A GraphQL field is only valid if all supplied arguments are defined by
 * that field.
 */
function KnownArgumentNamesRule(context) {
  return _objectSpread(_objectSpread({}, KnownArgumentNamesOnDirectivesRule(context)), {}, {
    Argument: function Argument(argNode) {
      var argDef = context.getArgument();
      var fieldDef = context.getFieldDef();
      var parentType = context.getParentType();

      if (!argDef && fieldDef && parentType) {
        var argName = argNode.name.value;
        var knownArgsNames = fieldDef.args.map(function (arg) {
          return arg.name;
        });
        var suggestions = (0, _suggestionList.default)(argName, knownArgsNames);
        context.reportError(new _GraphQLError.GraphQLError("Unknown argument \"".concat(argName, "\" on field \"").concat(parentType.name, ".").concat(fieldDef.name, "\".") + (0, _didYouMean.default)(suggestions), argNode));
      }
    }
  });
}
/**
 * @internal
 */


function KnownArgumentNamesOnDirectivesRule(context) {
  var directiveArgs = Object.create(null);
  var schema = context.getSchema();
  var definedDirectives = schema ? schema.getDirectives() : _directives.specifiedDirectives;

  for (var _i2 = 0; _i2 < definedDirectives.length; _i2++) {
    var directive = definedDirectives[_i2];
    directiveArgs[directive.name] = directive.args.map(function (arg) {
      return arg.name;
    });
  }

  var astDefinitions = context.getDocument().definitions;

  for (var _i4 = 0; _i4 < astDefinitions.length; _i4++) {
    var def = astDefinitions[_i4];

    if (def.kind === _kinds.Kind.DIRECTIVE_DEFINITION) {
      var _def$arguments;

      // istanbul ignore next (See: 'https://github.com/graphql/graphql-js/issues/2203')
      var argsNodes = (_def$arguments = def.arguments) !== null && _def$arguments !== void 0 ? _def$arguments : [];
      directiveArgs[def.name.value] = argsNodes.map(function (arg) {
        return arg.name.value;
      });
    }
  }

  return {
    Directive: function Directive(directiveNode) {
      var directiveName = directiveNode.name.value;
      var knownArgs = directiveArgs[directiveName];

      if (directiveNode.arguments && knownArgs) {
        for (var _i6 = 0, _directiveNode$argume2 = directiveNode.arguments; _i6 < _directiveNode$argume2.length; _i6++) {
          var argNode = _directiveNode$argume2[_i6];
          var argName = argNode.name.value;

          if (knownArgs.indexOf(argName) === -1) {
            var suggestions = (0, _suggestionList.default)(argName, knownArgs);
            context.reportError(new _GraphQLError.GraphQLError("Unknown argument \"".concat(argName, "\" on directive \"@").concat(directiveName, "\".") + (0, _didYouMean.default)(suggestions), argNode));
          }
        }
      }

      return false;
    }
  };
}
