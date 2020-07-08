// Spec Section: "Executable Definitions"
import { ExecutableDefinitionsRule } from "./rules/ExecutableDefinitionsRule.mjs"; // Spec Section: "Operation Name Uniqueness"

import { UniqueOperationNamesRule } from "./rules/UniqueOperationNamesRule.mjs"; // Spec Section: "Lone Anonymous Operation"

import { LoneAnonymousOperationRule } from "./rules/LoneAnonymousOperationRule.mjs"; // Spec Section: "Subscriptions with Single Root Field"

import { SingleFieldSubscriptionsRule } from "./rules/SingleFieldSubscriptionsRule.mjs"; // Spec Section: "Fragment Spread Type Existence"

import { KnownTypeNamesRule } from "./rules/KnownTypeNamesRule.mjs"; // Spec Section: "Fragments on Composite Types"

import { FragmentsOnCompositeTypesRule } from "./rules/FragmentsOnCompositeTypesRule.mjs"; // Spec Section: "Variables are Input Types"

import { VariablesAreInputTypesRule } from "./rules/VariablesAreInputTypesRule.mjs"; // Spec Section: "Leaf Field Selections"

import { ScalarLeafsRule } from "./rules/ScalarLeafsRule.mjs"; // Spec Section: "Field Selections on Objects, Interfaces, and Unions Types"

import { FieldsOnCorrectTypeRule } from "./rules/FieldsOnCorrectTypeRule.mjs"; // Spec Section: "Fragment Name Uniqueness"

import { UniqueFragmentNamesRule } from "./rules/UniqueFragmentNamesRule.mjs"; // Spec Section: "Fragment spread target defined"

import { KnownFragmentNamesRule } from "./rules/KnownFragmentNamesRule.mjs"; // Spec Section: "Fragments must be used"

import { NoUnusedFragmentsRule } from "./rules/NoUnusedFragmentsRule.mjs"; // Spec Section: "Fragment spread is possible"

import { PossibleFragmentSpreadsRule } from "./rules/PossibleFragmentSpreadsRule.mjs"; // Spec Section: "Fragments must not form cycles"

import { NoFragmentCyclesRule } from "./rules/NoFragmentCyclesRule.mjs"; // Spec Section: "Variable Uniqueness"

import { UniqueVariableNamesRule } from "./rules/UniqueVariableNamesRule.mjs"; // Spec Section: "All Variable Used Defined"

import { NoUndefinedVariablesRule } from "./rules/NoUndefinedVariablesRule.mjs"; // Spec Section: "All Variables Used"

import { NoUnusedVariablesRule } from "./rules/NoUnusedVariablesRule.mjs"; // Spec Section: "Directives Are Defined"

import { KnownDirectivesRule } from "./rules/KnownDirectivesRule.mjs"; // Spec Section: "Directives Are Unique Per Location"

import { UniqueDirectivesPerLocationRule } from "./rules/UniqueDirectivesPerLocationRule.mjs"; // Spec Section: "Argument Names"

import { KnownArgumentNamesRule, KnownArgumentNamesOnDirectivesRule } from "./rules/KnownArgumentNamesRule.mjs"; // Spec Section: "Argument Uniqueness"

import { UniqueArgumentNamesRule } from "./rules/UniqueArgumentNamesRule.mjs"; // Spec Section: "Value Type Correctness"

import { ValuesOfCorrectTypeRule } from "./rules/ValuesOfCorrectTypeRule.mjs"; // Spec Section: "Argument Optionality"

import { ProvidedRequiredArgumentsRule, ProvidedRequiredArgumentsOnDirectivesRule } from "./rules/ProvidedRequiredArgumentsRule.mjs"; // Spec Section: "All Variable Usages Are Allowed"

import { VariablesInAllowedPositionRule } from "./rules/VariablesInAllowedPositionRule.mjs"; // Spec Section: "Field Selection Merging"

import { OverlappingFieldsCanBeMergedRule } from "./rules/OverlappingFieldsCanBeMergedRule.mjs"; // Spec Section: "Input Object Field Uniqueness"

import { UniqueInputFieldNamesRule } from "./rules/UniqueInputFieldNamesRule.mjs"; // SDL-specific validation rules

import { LoneSchemaDefinitionRule } from "./rules/LoneSchemaDefinitionRule.mjs";
import { UniqueOperationTypesRule } from "./rules/UniqueOperationTypesRule.mjs";
import { UniqueTypeNamesRule } from "./rules/UniqueTypeNamesRule.mjs";
import { UniqueEnumValueNamesRule } from "./rules/UniqueEnumValueNamesRule.mjs";
import { UniqueFieldDefinitionNamesRule } from "./rules/UniqueFieldDefinitionNamesRule.mjs";
import { UniqueDirectiveNamesRule } from "./rules/UniqueDirectiveNamesRule.mjs";
import { PossibleTypeExtensionsRule } from "./rules/PossibleTypeExtensionsRule.mjs";
/**
 * This set includes all validation rules defined by the GraphQL spec.
 *
 * The order of the rules in this list has been adjusted to lead to the
 * most clear output when encountering multiple validation errors.
 */

export var specifiedRules = Object.freeze([ExecutableDefinitionsRule, UniqueOperationNamesRule, LoneAnonymousOperationRule, SingleFieldSubscriptionsRule, KnownTypeNamesRule, FragmentsOnCompositeTypesRule, VariablesAreInputTypesRule, ScalarLeafsRule, FieldsOnCorrectTypeRule, UniqueFragmentNamesRule, KnownFragmentNamesRule, NoUnusedFragmentsRule, PossibleFragmentSpreadsRule, NoFragmentCyclesRule, UniqueVariableNamesRule, NoUndefinedVariablesRule, NoUnusedVariablesRule, KnownDirectivesRule, UniqueDirectivesPerLocationRule, KnownArgumentNamesRule, UniqueArgumentNamesRule, ValuesOfCorrectTypeRule, ProvidedRequiredArgumentsRule, VariablesInAllowedPositionRule, OverlappingFieldsCanBeMergedRule, UniqueInputFieldNamesRule]);
/**
 * @internal
 */

export var specifiedSDLRules = Object.freeze([LoneSchemaDefinitionRule, UniqueOperationTypesRule, UniqueTypeNamesRule, UniqueEnumValueNamesRule, UniqueFieldDefinitionNamesRule, UniqueDirectiveNamesRule, KnownTypeNamesRule, KnownDirectivesRule, UniqueDirectivesPerLocationRule, PossibleTypeExtensionsRule, KnownArgumentNamesOnDirectivesRule, UniqueArgumentNamesRule, UniqueInputFieldNamesRule, ProvidedRequiredArgumentsOnDirectivesRule]);
