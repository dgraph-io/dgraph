import devAssert from "../jsutils/devAssert.mjs";
import { GraphQLError } from "../error/GraphQLError.mjs";
import { visit, visitInParallel } from "../language/visitor.mjs";
import { assertValidSchema } from "../type/validate.mjs";
import { TypeInfo, visitWithTypeInfo } from "../utilities/TypeInfo.mjs";
import { specifiedRules, specifiedSDLRules } from "./specifiedRules.mjs";
import { SDLValidationContext, ValidationContext } from "./ValidationContext.mjs";
/**
 * Implements the "Validation" section of the spec.
 *
 * Validation runs synchronously, returning an array of encountered errors, or
 * an empty array if no errors were encountered and the document is valid.
 *
 * A list of specific validation rules may be provided. If not provided, the
 * default list of rules defined by the GraphQL specification will be used.
 *
 * Each validation rules is a function which returns a visitor
 * (see the language/visitor API). Visitor methods are expected to return
 * GraphQLErrors, or Arrays of GraphQLErrors when invalid.
 *
 * Optionally a custom TypeInfo instance may be provided. If not provided, one
 * will be created from the provided schema.
 */

export function validate(schema, documentAST) {
  var rules = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : specifiedRules;
  var typeInfo = arguments.length > 3 && arguments[3] !== undefined ? arguments[3] : new TypeInfo(schema);
  var options = arguments.length > 4 && arguments[4] !== undefined ? arguments[4] : {
    maxErrors: undefined
  };
  documentAST || devAssert(0, 'Must provide document.'); // If the schema used for validation is invalid, throw an error.

  assertValidSchema(schema);
  var abortObj = Object.freeze({});
  var errors = [];
  var context = new ValidationContext(schema, documentAST, typeInfo, function (error) {
    if (options.maxErrors != null && errors.length >= options.maxErrors) {
      errors.push(new GraphQLError('Too many validation errors, error limit reached. Validation aborted.'));
      throw abortObj;
    }

    errors.push(error);
  }); // This uses a specialized visitor which runs multiple visitors in parallel,
  // while maintaining the visitor skip and break API.

  var visitor = visitInParallel(rules.map(function (rule) {
    return rule(context);
  })); // Visit the whole document with each instance of all provided rules.

  try {
    visit(documentAST, visitWithTypeInfo(typeInfo, visitor));
  } catch (e) {
    if (e !== abortObj) {
      throw e;
    }
  }

  return errors;
}
/**
 * @internal
 */

export function validateSDL(documentAST, schemaToExtend) {
  var rules = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : specifiedSDLRules;
  var errors = [];
  var context = new SDLValidationContext(documentAST, schemaToExtend, function (error) {
    errors.push(error);
  });
  var visitors = rules.map(function (rule) {
    return rule(context);
  });
  visit(documentAST, visitInParallel(visitors));
  return errors;
}
/**
 * Utility function which asserts a SDL document is valid by throwing an error
 * if it is invalid.
 *
 * @internal
 */

export function assertValidSDL(documentAST) {
  var errors = validateSDL(documentAST);

  if (errors.length !== 0) {
    throw new Error(errors.map(function (error) {
      return error.message;
    }).join('\n\n'));
  }
}
/**
 * Utility function which asserts a SDL document is valid by throwing an error
 * if it is invalid.
 *
 * @internal
 */

export function assertValidSDLExtension(documentAST, schema) {
  var errors = validateSDL(documentAST, schema);

  if (errors.length !== 0) {
    throw new Error(errors.map(function (error) {
      return error.message;
    }).join('\n\n'));
  }
}
