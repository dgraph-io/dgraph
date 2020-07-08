import { GraphQLError } from "../../../error/GraphQLError.mjs";
import { getNamedType } from "../../../type/definition.mjs";

/**
 * No deprecated
 *
 * A GraphQL document is only valid if all selected fields and all used enum values have not been
 * deprecated.
 *
 * Note: This rule is optional and is not part of the Validation section of the GraphQL
 * Specification. The main purpose of this rule is detection of deprecated usages and not
 * necessarily to forbid their use when querying a service.
 */
export function NoDeprecatedCustomRule(context) {
  return {
    Field: function Field(node) {
      var fieldDef = context.getFieldDef();
      var parentType = context.getParentType();

      if (parentType && (fieldDef === null || fieldDef === void 0 ? void 0 : fieldDef.deprecationReason) != null) {
        context.reportError(new GraphQLError("The field ".concat(parentType.name, ".").concat(fieldDef.name, " is deprecated. ") + fieldDef.deprecationReason, node));
      }
    },
    EnumValue: function EnumValue(node) {
      var type = getNamedType(context.getInputType());
      var enumValue = context.getEnumValue();

      if (type && (enumValue === null || enumValue === void 0 ? void 0 : enumValue.deprecationReason) != null) {
        context.reportError(new GraphQLError("The enum value \"".concat(type.name, ".").concat(enumValue.name, "\" is deprecated. ") + enumValue.deprecationReason, node));
      }
    }
  };
}
