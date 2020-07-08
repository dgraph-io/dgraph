import { GraphQLError } from "../../error/GraphQLError.mjs";
import { print } from "../../language/printer.mjs";
import { isInputType } from "../../type/definition.mjs";
import { typeFromAST } from "../../utilities/typeFromAST.mjs";

/**
 * Variables are input types
 *
 * A GraphQL operation is only valid if all the variables it defines are of
 * input types (scalar, enum, or input object).
 */
export function VariablesAreInputTypesRule(context) {
  return {
    VariableDefinition: function VariableDefinition(node) {
      var type = typeFromAST(context.getSchema(), node.type);

      if (type && !isInputType(type)) {
        var variableName = node.variable.name.value;
        var typeName = print(node.type);
        context.reportError(new GraphQLError("Variable \"$".concat(variableName, "\" cannot be non-input type \"").concat(typeName, "\"."), node.type));
      }
    }
  };
}
