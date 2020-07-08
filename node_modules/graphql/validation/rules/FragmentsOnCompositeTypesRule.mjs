import { GraphQLError } from "../../error/GraphQLError.mjs";
import { print } from "../../language/printer.mjs";
import { isCompositeType } from "../../type/definition.mjs";
import { typeFromAST } from "../../utilities/typeFromAST.mjs";

/**
 * Fragments on composite type
 *
 * Fragments use a type condition to determine if they apply, since fragments
 * can only be spread into a composite type (object, interface, or union), the
 * type condition must also be a composite type.
 */
export function FragmentsOnCompositeTypesRule(context) {
  return {
    InlineFragment: function InlineFragment(node) {
      var typeCondition = node.typeCondition;

      if (typeCondition) {
        var type = typeFromAST(context.getSchema(), typeCondition);

        if (type && !isCompositeType(type)) {
          var typeStr = print(typeCondition);
          context.reportError(new GraphQLError("Fragment cannot condition on non composite type \"".concat(typeStr, "\"."), typeCondition));
        }
      }
    },
    FragmentDefinition: function FragmentDefinition(node) {
      var type = typeFromAST(context.getSchema(), node.typeCondition);

      if (type && !isCompositeType(type)) {
        var typeStr = print(node.typeCondition);
        context.reportError(new GraphQLError("Fragment \"".concat(node.name.value, "\" cannot condition on non composite type \"").concat(typeStr, "\"."), node.typeCondition));
      }
    }
  };
}
