"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.isDirective = isDirective;
exports.assertDirective = assertDirective;
exports.isSpecifiedDirective = isSpecifiedDirective;
exports.specifiedDirectives = exports.GraphQLSpecifiedByDirective = exports.GraphQLDeprecatedDirective = exports.DEFAULT_DEPRECATION_REASON = exports.GraphQLSkipDirective = exports.GraphQLIncludeDirective = exports.GraphQLDirective = void 0;

var _objectEntries = _interopRequireDefault(require("../polyfills/objectEntries"));

var _symbols = require("../polyfills/symbols");

var _inspect = _interopRequireDefault(require("../jsutils/inspect"));

var _toObjMap = _interopRequireDefault(require("../jsutils/toObjMap"));

var _devAssert = _interopRequireDefault(require("../jsutils/devAssert"));

var _instanceOf = _interopRequireDefault(require("../jsutils/instanceOf"));

var _isObjectLike = _interopRequireDefault(require("../jsutils/isObjectLike"));

var _defineInspect = _interopRequireDefault(require("../jsutils/defineInspect"));

var _directiveLocation = require("../language/directiveLocation");

var _scalars = require("./scalars");

var _definition = require("./definition");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

function _defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function _createClass(Constructor, protoProps, staticProps) { if (protoProps) _defineProperties(Constructor.prototype, protoProps); if (staticProps) _defineProperties(Constructor, staticProps); return Constructor; }

// eslint-disable-next-line no-redeclare
function isDirective(directive) {
  return (0, _instanceOf.default)(directive, GraphQLDirective);
}

function assertDirective(directive) {
  if (!isDirective(directive)) {
    throw new Error("Expected ".concat((0, _inspect.default)(directive), " to be a GraphQL directive."));
  }

  return directive;
}
/**
 * Directives are used by the GraphQL runtime as a way of modifying execution
 * behavior. Type system creators will usually not create these directly.
 */


var GraphQLDirective = /*#__PURE__*/function () {
  function GraphQLDirective(config) {
    var _config$isRepeatable, _config$args;

    this.name = config.name;
    this.description = config.description;
    this.locations = config.locations;
    this.isRepeatable = (_config$isRepeatable = config.isRepeatable) !== null && _config$isRepeatable !== void 0 ? _config$isRepeatable : false;
    this.extensions = config.extensions && (0, _toObjMap.default)(config.extensions);
    this.astNode = config.astNode;
    config.name || (0, _devAssert.default)(0, 'Directive must be named.');
    Array.isArray(config.locations) || (0, _devAssert.default)(0, "@".concat(config.name, " locations must be an Array."));
    var args = (_config$args = config.args) !== null && _config$args !== void 0 ? _config$args : {};
    (0, _isObjectLike.default)(args) && !Array.isArray(args) || (0, _devAssert.default)(0, "@".concat(config.name, " args must be an object with argument names as keys."));
    this.args = (0, _objectEntries.default)(args).map(function (_ref) {
      var argName = _ref[0],
          argConfig = _ref[1];
      return {
        name: argName,
        description: argConfig.description,
        type: argConfig.type,
        defaultValue: argConfig.defaultValue,
        extensions: argConfig.extensions && (0, _toObjMap.default)(argConfig.extensions),
        astNode: argConfig.astNode
      };
    });
  }

  var _proto = GraphQLDirective.prototype;

  _proto.toConfig = function toConfig() {
    return {
      name: this.name,
      description: this.description,
      locations: this.locations,
      args: (0, _definition.argsToArgsConfig)(this.args),
      isRepeatable: this.isRepeatable,
      extensions: this.extensions,
      astNode: this.astNode
    };
  };

  _proto.toString = function toString() {
    return '@' + this.name;
  };

  _proto.toJSON = function toJSON() {
    return this.toString();
  } // $FlowFixMe Flow doesn't support computed properties yet
  ;

  _createClass(GraphQLDirective, [{
    key: _symbols.SYMBOL_TO_STRING_TAG,
    get: function get() {
      return 'GraphQLDirective';
    }
  }]);

  return GraphQLDirective;
}(); // Print a simplified form when appearing in `inspect` and `util.inspect`.


exports.GraphQLDirective = GraphQLDirective;
(0, _defineInspect.default)(GraphQLDirective);

/**
 * Used to conditionally include fields or fragments.
 */
var GraphQLIncludeDirective = new GraphQLDirective({
  name: 'include',
  description: 'Directs the executor to include this field or fragment only when the `if` argument is true.',
  locations: [_directiveLocation.DirectiveLocation.FIELD, _directiveLocation.DirectiveLocation.FRAGMENT_SPREAD, _directiveLocation.DirectiveLocation.INLINE_FRAGMENT],
  args: {
    if: {
      type: (0, _definition.GraphQLNonNull)(_scalars.GraphQLBoolean),
      description: 'Included when true.'
    }
  }
});
/**
 * Used to conditionally skip (exclude) fields or fragments.
 */

exports.GraphQLIncludeDirective = GraphQLIncludeDirective;
var GraphQLSkipDirective = new GraphQLDirective({
  name: 'skip',
  description: 'Directs the executor to skip this field or fragment when the `if` argument is true.',
  locations: [_directiveLocation.DirectiveLocation.FIELD, _directiveLocation.DirectiveLocation.FRAGMENT_SPREAD, _directiveLocation.DirectiveLocation.INLINE_FRAGMENT],
  args: {
    if: {
      type: (0, _definition.GraphQLNonNull)(_scalars.GraphQLBoolean),
      description: 'Skipped when true.'
    }
  }
});
/**
 * Constant string used for default reason for a deprecation.
 */

exports.GraphQLSkipDirective = GraphQLSkipDirective;
var DEFAULT_DEPRECATION_REASON = 'No longer supported';
/**
 * Used to declare element of a GraphQL schema as deprecated.
 */

exports.DEFAULT_DEPRECATION_REASON = DEFAULT_DEPRECATION_REASON;
var GraphQLDeprecatedDirective = new GraphQLDirective({
  name: 'deprecated',
  description: 'Marks an element of a GraphQL schema as no longer supported.',
  locations: [_directiveLocation.DirectiveLocation.FIELD_DEFINITION, _directiveLocation.DirectiveLocation.ENUM_VALUE],
  args: {
    reason: {
      type: _scalars.GraphQLString,
      description: 'Explains why this element was deprecated, usually also including a suggestion for how to access supported similar data. Formatted using the Markdown syntax, as specified by [CommonMark](https://commonmark.org/).',
      defaultValue: DEFAULT_DEPRECATION_REASON
    }
  }
});
/**
 * Used to provide a URL for specifying the behaviour of custom scalar definitions.
 */

exports.GraphQLDeprecatedDirective = GraphQLDeprecatedDirective;
var GraphQLSpecifiedByDirective = new GraphQLDirective({
  name: 'specifiedBy',
  description: 'Exposes a URL that specifies the behaviour of this scalar.',
  locations: [_directiveLocation.DirectiveLocation.SCALAR],
  args: {
    url: {
      type: (0, _definition.GraphQLNonNull)(_scalars.GraphQLString),
      description: 'The URL that specifies the behaviour of this scalar.'
    }
  }
});
/**
 * The full list of specified directives.
 */

exports.GraphQLSpecifiedByDirective = GraphQLSpecifiedByDirective;
var specifiedDirectives = Object.freeze([GraphQLIncludeDirective, GraphQLSkipDirective, GraphQLDeprecatedDirective, GraphQLSpecifiedByDirective]);
exports.specifiedDirectives = specifiedDirectives;

function isSpecifiedDirective(directive) {
  return specifiedDirectives.some(function (_ref2) {
    var name = _ref2.name;
    return name === directive.name;
  });
}
