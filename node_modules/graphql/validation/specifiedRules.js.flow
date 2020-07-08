// @flow strict

// Spec Section: "Executable Definitions"
import { ExecutableDefinitionsRule } from './rules/ExecutableDefinitionsRule';

// Spec Section: "Operation Name Uniqueness"
import { UniqueOperationNamesRule } from './rules/UniqueOperationNamesRule';

// Spec Section: "Lone Anonymous Operation"
import { LoneAnonymousOperationRule } from './rules/LoneAnonymousOperationRule';

// Spec Section: "Subscriptions with Single Root Field"
import { SingleFieldSubscriptionsRule } from './rules/SingleFieldSubscriptionsRule';

// Spec Section: "Fragment Spread Type Existence"
import { KnownTypeNamesRule } from './rules/KnownTypeNamesRule';

// Spec Section: "Fragments on Composite Types"
import { FragmentsOnCompositeTypesRule } from './rules/FragmentsOnCompositeTypesRule';

// Spec Section: "Variables are Input Types"
import { VariablesAreInputTypesRule } from './rules/VariablesAreInputTypesRule';

// Spec Section: "Leaf Field Selections"
import { ScalarLeafsRule } from './rules/ScalarLeafsRule';

// Spec Section: "Field Selections on Objects, Interfaces, and Unions Types"
import { FieldsOnCorrectTypeRule } from './rules/FieldsOnCorrectTypeRule';

// Spec Section: "Fragment Name Uniqueness"
import { UniqueFragmentNamesRule } from './rules/UniqueFragmentNamesRule';

// Spec Section: "Fragment spread target defined"
import { KnownFragmentNamesRule } from './rules/KnownFragmentNamesRule';

// Spec Section: "Fragments must be used"
import { NoUnusedFragmentsRule } from './rules/NoUnusedFragmentsRule';

// Spec Section: "Fragment spread is possible"
import { PossibleFragmentSpreadsRule } from './rules/PossibleFragmentSpreadsRule';

// Spec Section: "Fragments must not form cycles"
import { NoFragmentCyclesRule } from './rules/NoFragmentCyclesRule';

// Spec Section: "Variable Uniqueness"
import { UniqueVariableNamesRule } from './rules/UniqueVariableNamesRule';

// Spec Section: "All Variable Used Defined"
import { NoUndefinedVariablesRule } from './rules/NoUndefinedVariablesRule';

// Spec Section: "All Variables Used"
import { NoUnusedVariablesRule } from './rules/NoUnusedVariablesRule';

// Spec Section: "Directives Are Defined"
import { KnownDirectivesRule } from './rules/KnownDirectivesRule';

// Spec Section: "Directives Are Unique Per Location"
import { UniqueDirectivesPerLocationRule } from './rules/UniqueDirectivesPerLocationRule';

// Spec Section: "Argument Names"
import {
  KnownArgumentNamesRule,
  KnownArgumentNamesOnDirectivesRule,
} from './rules/KnownArgumentNamesRule';

// Spec Section: "Argument Uniqueness"
import { UniqueArgumentNamesRule } from './rules/UniqueArgumentNamesRule';

// Spec Section: "Value Type Correctness"
import { ValuesOfCorrectTypeRule } from './rules/ValuesOfCorrectTypeRule';

// Spec Section: "Argument Optionality"
import {
  ProvidedRequiredArgumentsRule,
  ProvidedRequiredArgumentsOnDirectivesRule,
} from './rules/ProvidedRequiredArgumentsRule';

// Spec Section: "All Variable Usages Are Allowed"
import { VariablesInAllowedPositionRule } from './rules/VariablesInAllowedPositionRule';

// Spec Section: "Field Selection Merging"
import { OverlappingFieldsCanBeMergedRule } from './rules/OverlappingFieldsCanBeMergedRule';

// Spec Section: "Input Object Field Uniqueness"
import { UniqueInputFieldNamesRule } from './rules/UniqueInputFieldNamesRule';

// SDL-specific validation rules
import { LoneSchemaDefinitionRule } from './rules/LoneSchemaDefinitionRule';
import { UniqueOperationTypesRule } from './rules/UniqueOperationTypesRule';
import { UniqueTypeNamesRule } from './rules/UniqueTypeNamesRule';
import { UniqueEnumValueNamesRule } from './rules/UniqueEnumValueNamesRule';
import { UniqueFieldDefinitionNamesRule } from './rules/UniqueFieldDefinitionNamesRule';
import { UniqueDirectiveNamesRule } from './rules/UniqueDirectiveNamesRule';
import { PossibleTypeExtensionsRule } from './rules/PossibleTypeExtensionsRule';

/**
 * This set includes all validation rules defined by the GraphQL spec.
 *
 * The order of the rules in this list has been adjusted to lead to the
 * most clear output when encountering multiple validation errors.
 */
export const specifiedRules = Object.freeze([
  ExecutableDefinitionsRule,
  UniqueOperationNamesRule,
  LoneAnonymousOperationRule,
  SingleFieldSubscriptionsRule,
  KnownTypeNamesRule,
  FragmentsOnCompositeTypesRule,
  VariablesAreInputTypesRule,
  ScalarLeafsRule,
  FieldsOnCorrectTypeRule,
  UniqueFragmentNamesRule,
  KnownFragmentNamesRule,
  NoUnusedFragmentsRule,
  PossibleFragmentSpreadsRule,
  NoFragmentCyclesRule,
  UniqueVariableNamesRule,
  NoUndefinedVariablesRule,
  NoUnusedVariablesRule,
  KnownDirectivesRule,
  UniqueDirectivesPerLocationRule,
  KnownArgumentNamesRule,
  UniqueArgumentNamesRule,
  ValuesOfCorrectTypeRule,
  ProvidedRequiredArgumentsRule,
  VariablesInAllowedPositionRule,
  OverlappingFieldsCanBeMergedRule,
  UniqueInputFieldNamesRule,
]);

/**
 * @internal
 */
export const specifiedSDLRules = Object.freeze([
  LoneSchemaDefinitionRule,
  UniqueOperationTypesRule,
  UniqueTypeNamesRule,
  UniqueEnumValueNamesRule,
  UniqueFieldDefinitionNamesRule,
  UniqueDirectiveNamesRule,
  KnownTypeNamesRule,
  KnownDirectivesRule,
  UniqueDirectivesPerLocationRule,
  PossibleTypeExtensionsRule,
  KnownArgumentNamesOnDirectivesRule,
  UniqueArgumentNamesRule,
  UniqueInputFieldNamesRule,
  ProvidedRequiredArgumentsOnDirectivesRule,
]);
