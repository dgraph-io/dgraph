"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.printSchema = printSchema;
exports.printIntrospectionSchema = printIntrospectionSchema;
exports.printType = printType;

var _objectValues = _interopRequireDefault(require("../polyfills/objectValues"));

var _inspect = _interopRequireDefault(require("../jsutils/inspect"));

var _invariant = _interopRequireDefault(require("../jsutils/invariant"));

var _printer = require("../language/printer");

var _blockString = require("../language/blockString");

var _introspection = require("../type/introspection");

var _scalars = require("../type/scalars");

var _directives = require("../type/directives");

var _definition = require("../type/definition");

var _astFromValue = require("./astFromValue");

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 * Accepts options as a second argument:
 *
 *    - commentDescriptions:
 *        Provide true to use preceding comments as the description.
 *
 */
function printSchema(schema, options) {
  return printFilteredSchema(schema, function (n) {
    return !(0, _directives.isSpecifiedDirective)(n);
  }, isDefinedType, options);
}

function printIntrospectionSchema(schema, options) {
  return printFilteredSchema(schema, _directives.isSpecifiedDirective, _introspection.isIntrospectionType, options);
}

function isDefinedType(type) {
  return !(0, _scalars.isSpecifiedScalarType)(type) && !(0, _introspection.isIntrospectionType)(type);
}

function printFilteredSchema(schema, directiveFilter, typeFilter, options) {
  var directives = schema.getDirectives().filter(directiveFilter);
  var types = (0, _objectValues.default)(schema.getTypeMap()).filter(typeFilter);
  return [printSchemaDefinition(schema)].concat(directives.map(function (directive) {
    return printDirective(directive, options);
  }), types.map(function (type) {
    return printType(type, options);
  })).filter(Boolean).join('\n\n') + '\n';
}

function printSchemaDefinition(schema) {
  if (schema.description == null && isSchemaOfCommonNames(schema)) {
    return;
  }

  var operationTypes = [];
  var queryType = schema.getQueryType();

  if (queryType) {
    operationTypes.push("  query: ".concat(queryType.name));
  }

  var mutationType = schema.getMutationType();

  if (mutationType) {
    operationTypes.push("  mutation: ".concat(mutationType.name));
  }

  var subscriptionType = schema.getSubscriptionType();

  if (subscriptionType) {
    operationTypes.push("  subscription: ".concat(subscriptionType.name));
  }

  return printDescription({}, schema) + "schema {\n".concat(operationTypes.join('\n'), "\n}");
}
/**
 * GraphQL schema define root types for each type of operation. These types are
 * the same as any other type and can be named in any manner, however there is
 * a common naming convention:
 *
 *   schema {
 *     query: Query
 *     mutation: Mutation
 *   }
 *
 * When using this naming convention, the schema description can be omitted.
 */


function isSchemaOfCommonNames(schema) {
  var queryType = schema.getQueryType();

  if (queryType && queryType.name !== 'Query') {
    return false;
  }

  var mutationType = schema.getMutationType();

  if (mutationType && mutationType.name !== 'Mutation') {
    return false;
  }

  var subscriptionType = schema.getSubscriptionType();

  if (subscriptionType && subscriptionType.name !== 'Subscription') {
    return false;
  }

  return true;
}

function printType(type, options) {
  if ((0, _definition.isScalarType)(type)) {
    return printScalar(type, options);
  }

  if ((0, _definition.isObjectType)(type)) {
    return printObject(type, options);
  }

  if ((0, _definition.isInterfaceType)(type)) {
    return printInterface(type, options);
  }

  if ((0, _definition.isUnionType)(type)) {
    return printUnion(type, options);
  }

  if ((0, _definition.isEnumType)(type)) {
    return printEnum(type, options);
  } // istanbul ignore else (See: 'https://github.com/graphql/graphql-js/issues/2618')


  if ((0, _definition.isInputObjectType)(type)) {
    return printInputObject(type, options);
  } // istanbul ignore next (Not reachable. All possible types have been considered)


  false || (0, _invariant.default)(0, 'Unexpected type: ' + (0, _inspect.default)(type));
}

function printScalar(type, options) {
  return printDescription(options, type) + "scalar ".concat(type.name) + printSpecifiedByUrl(type);
}

function printImplementedInterfaces(type) {
  var interfaces = type.getInterfaces();
  return interfaces.length ? ' implements ' + interfaces.map(function (i) {
    return i.name;
  }).join(' & ') : '';
}

function printObject(type, options) {
  return printDescription(options, type) + "type ".concat(type.name) + printImplementedInterfaces(type) + printFields(options, type);
}

function printInterface(type, options) {
  return printDescription(options, type) + "interface ".concat(type.name) + printImplementedInterfaces(type) + printFields(options, type);
}

function printUnion(type, options) {
  var types = type.getTypes();
  var possibleTypes = types.length ? ' = ' + types.join(' | ') : '';
  return printDescription(options, type) + 'union ' + type.name + possibleTypes;
}

function printEnum(type, options) {
  var values = type.getValues().map(function (value, i) {
    return printDescription(options, value, '  ', !i) + '  ' + value.name + printDeprecated(value);
  });
  return printDescription(options, type) + "enum ".concat(type.name) + printBlock(values);
}

function printInputObject(type, options) {
  var fields = (0, _objectValues.default)(type.getFields()).map(function (f, i) {
    return printDescription(options, f, '  ', !i) + '  ' + printInputValue(f);
  });
  return printDescription(options, type) + "input ".concat(type.name) + printBlock(fields);
}

function printFields(options, type) {
  var fields = (0, _objectValues.default)(type.getFields()).map(function (f, i) {
    return printDescription(options, f, '  ', !i) + '  ' + f.name + printArgs(options, f.args, '  ') + ': ' + String(f.type) + printDeprecated(f);
  });
  return printBlock(fields);
}

function printBlock(items) {
  return items.length !== 0 ? ' {\n' + items.join('\n') + '\n}' : '';
}

function printArgs(options, args) {
  var indentation = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : '';

  if (args.length === 0) {
    return '';
  } // If every arg does not have a description, print them on one line.


  if (args.every(function (arg) {
    return !arg.description;
  })) {
    return '(' + args.map(printInputValue).join(', ') + ')';
  }

  return '(\n' + args.map(function (arg, i) {
    return printDescription(options, arg, '  ' + indentation, !i) + '  ' + indentation + printInputValue(arg);
  }).join('\n') + '\n' + indentation + ')';
}

function printInputValue(arg) {
  var defaultAST = (0, _astFromValue.astFromValue)(arg.defaultValue, arg.type);
  var argDecl = arg.name + ': ' + String(arg.type);

  if (defaultAST) {
    argDecl += " = ".concat((0, _printer.print)(defaultAST));
  }

  return argDecl;
}

function printDirective(directive, options) {
  return printDescription(options, directive) + 'directive @' + directive.name + printArgs(options, directive.args) + (directive.isRepeatable ? ' repeatable' : '') + ' on ' + directive.locations.join(' | ');
}

function printDeprecated(fieldOrEnumVal) {
  if (!fieldOrEnumVal.isDeprecated) {
    return '';
  }

  var reason = fieldOrEnumVal.deprecationReason;
  var reasonAST = (0, _astFromValue.astFromValue)(reason, _scalars.GraphQLString);

  if (reasonAST && reason !== _directives.DEFAULT_DEPRECATION_REASON) {
    return ' @deprecated(reason: ' + (0, _printer.print)(reasonAST) + ')';
  }

  return ' @deprecated';
}

function printSpecifiedByUrl(scalar) {
  if (scalar.specifiedByUrl == null) {
    return '';
  }

  var url = scalar.specifiedByUrl;
  var urlAST = (0, _astFromValue.astFromValue)(url, _scalars.GraphQLString);
  urlAST || (0, _invariant.default)(0, 'Unexpected null value returned from `astFromValue` for specifiedByUrl');
  return ' @specifiedBy(url: ' + (0, _printer.print)(urlAST) + ')';
}

function printDescription(options, def) {
  var indentation = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : '';
  var firstInBlock = arguments.length > 3 && arguments[3] !== undefined ? arguments[3] : true;
  var description = def.description;

  if (description == null) {
    return '';
  }

  if ((options === null || options === void 0 ? void 0 : options.commentDescriptions) === true) {
    return printDescriptionWithComments(description, indentation, firstInBlock);
  }

  var preferMultipleLines = description.length > 70;
  var blockString = (0, _blockString.printBlockString)(description, '', preferMultipleLines);
  var prefix = indentation && !firstInBlock ? '\n' + indentation : indentation;
  return prefix + blockString.replace(/\n/g, '\n' + indentation) + '\n';
}

function printDescriptionWithComments(description, indentation, firstInBlock) {
  var prefix = indentation && !firstInBlock ? '\n' : '';
  var comment = description.split('\n').map(function (line) {
    return indentation + (line !== '' ? '# ' + line : '#');
  }).join('\n');
  return prefix + comment + '\n';
}
