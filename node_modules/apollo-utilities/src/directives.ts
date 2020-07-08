// Provides the methods that allow QueryManager to handle the `skip` and
// `include` directives within GraphQL.
import {
  FieldNode,
  SelectionNode,
  VariableNode,
  BooleanValueNode,
  DirectiveNode,
  DocumentNode,
  ArgumentNode,
  ValueNode,
} from 'graphql';

import { visit } from 'graphql/language/visitor';

import { invariant } from 'ts-invariant';

import { argumentsObjectFromField } from './storeUtils';

export type DirectiveInfo = {
  [fieldName: string]: { [argName: string]: any };
};

export function getDirectiveInfoFromField(
  field: FieldNode,
  variables: Object,
): DirectiveInfo {
  if (field.directives && field.directives.length) {
    const directiveObj: DirectiveInfo = {};
    field.directives.forEach((directive: DirectiveNode) => {
      directiveObj[directive.name.value] = argumentsObjectFromField(
        directive,
        variables,
      );
    });
    return directiveObj;
  }
  return null;
}

export function shouldInclude(
  selection: SelectionNode,
  variables: { [name: string]: any } = {},
): boolean {
  return getInclusionDirectives(
    selection.directives,
  ).every(({ directive, ifArgument }) => {
    let evaledValue: boolean = false;
    if (ifArgument.value.kind === 'Variable') {
      evaledValue = variables[(ifArgument.value as VariableNode).name.value];
      invariant(
        evaledValue !== void 0,
        `Invalid variable referenced in @${directive.name.value} directive.`,
      );
    } else {
      evaledValue = (ifArgument.value as BooleanValueNode).value;
    }
    return directive.name.value === 'skip' ? !evaledValue : evaledValue;
  });
}

export function getDirectiveNames(doc: DocumentNode) {
  const names: string[] = [];

  visit(doc, {
    Directive(node) {
      names.push(node.name.value);
    },
  });

  return names;
}

export function hasDirectives(names: string[], doc: DocumentNode) {
  return getDirectiveNames(doc).some(
    (name: string) => names.indexOf(name) > -1,
  );
}

export function hasClientExports(document: DocumentNode) {
  return (
    document &&
    hasDirectives(['client'], document) &&
    hasDirectives(['export'], document)
  );
}

export type InclusionDirectives = Array<{
  directive: DirectiveNode;
  ifArgument: ArgumentNode;
}>;

function isInclusionDirective({ name: { value } }: DirectiveNode): boolean {
  return value === 'skip' || value === 'include';
}

export function getInclusionDirectives(
  directives: ReadonlyArray<DirectiveNode>,
): InclusionDirectives {
  return directives ? directives.filter(isInclusionDirective).map(directive => {
    const directiveArguments = directive.arguments;
    const directiveName = directive.name.value;

    invariant(
      directiveArguments && directiveArguments.length === 1,
      `Incorrect number of arguments for the @${directiveName} directive.`,
    );

    const ifArgument = directiveArguments[0];
    invariant(
      ifArgument.name && ifArgument.name.value === 'if',
      `Invalid argument for the @${directiveName} directive.`,
    );

    const ifValue: ValueNode = ifArgument.value;

    // means it has to be a variable value if this is a valid @skip or @include directive
    invariant(
      ifValue &&
        (ifValue.kind === 'Variable' || ifValue.kind === 'BooleanValue'),
      `Argument for the @${directiveName} directive must be a variable or a boolean value.`,
    );

    return { directive, ifArgument };
  }) : [];
}

