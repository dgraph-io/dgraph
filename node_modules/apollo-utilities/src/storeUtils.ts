import {
  DirectiveNode,
  FieldNode,
  IntValueNode,
  FloatValueNode,
  StringValueNode,
  BooleanValueNode,
  ObjectValueNode,
  ListValueNode,
  EnumValueNode,
  NullValueNode,
  VariableNode,
  InlineFragmentNode,
  ValueNode,
  SelectionNode,
  NameNode,
} from 'graphql';

import stringify from 'fast-json-stable-stringify';
import { InvariantError } from 'ts-invariant';

export interface IdValue {
  type: 'id';
  id: string;
  generated: boolean;
  typename: string | undefined;
}

export interface JsonValue {
  type: 'json';
  json: any;
}

export type ListValue = Array<null | IdValue>;

export type StoreValue =
  | number
  | string
  | string[]
  | IdValue
  | ListValue
  | JsonValue
  | null
  | undefined
  | void
  | Object;

export type ScalarValue = StringValueNode | BooleanValueNode | EnumValueNode;

export function isScalarValue(value: ValueNode): value is ScalarValue {
  return ['StringValue', 'BooleanValue', 'EnumValue'].indexOf(value.kind) > -1;
}

export type NumberValue = IntValueNode | FloatValueNode;

export function isNumberValue(value: ValueNode): value is NumberValue {
  return ['IntValue', 'FloatValue'].indexOf(value.kind) > -1;
}

function isStringValue(value: ValueNode): value is StringValueNode {
  return value.kind === 'StringValue';
}

function isBooleanValue(value: ValueNode): value is BooleanValueNode {
  return value.kind === 'BooleanValue';
}

function isIntValue(value: ValueNode): value is IntValueNode {
  return value.kind === 'IntValue';
}

function isFloatValue(value: ValueNode): value is FloatValueNode {
  return value.kind === 'FloatValue';
}

function isVariable(value: ValueNode): value is VariableNode {
  return value.kind === 'Variable';
}

function isObjectValue(value: ValueNode): value is ObjectValueNode {
  return value.kind === 'ObjectValue';
}

function isListValue(value: ValueNode): value is ListValueNode {
  return value.kind === 'ListValue';
}

function isEnumValue(value: ValueNode): value is EnumValueNode {
  return value.kind === 'EnumValue';
}

function isNullValue(value: ValueNode): value is NullValueNode {
  return value.kind === 'NullValue';
}

export function valueToObjectRepresentation(
  argObj: any,
  name: NameNode,
  value: ValueNode,
  variables?: Object,
) {
  if (isIntValue(value) || isFloatValue(value)) {
    argObj[name.value] = Number(value.value);
  } else if (isBooleanValue(value) || isStringValue(value)) {
    argObj[name.value] = value.value;
  } else if (isObjectValue(value)) {
    const nestedArgObj = {};
    value.fields.map(obj =>
      valueToObjectRepresentation(nestedArgObj, obj.name, obj.value, variables),
    );
    argObj[name.value] = nestedArgObj;
  } else if (isVariable(value)) {
    const variableValue = (variables || ({} as any))[value.name.value];
    argObj[name.value] = variableValue;
  } else if (isListValue(value)) {
    argObj[name.value] = value.values.map(listValue => {
      const nestedArgArrayObj = {};
      valueToObjectRepresentation(
        nestedArgArrayObj,
        name,
        listValue,
        variables,
      );
      return (nestedArgArrayObj as any)[name.value];
    });
  } else if (isEnumValue(value)) {
    argObj[name.value] = (value as EnumValueNode).value;
  } else if (isNullValue(value)) {
    argObj[name.value] = null;
  } else {
    throw new InvariantError(
      `The inline argument "${name.value}" of kind "${(value as any).kind}"` +
        'is not supported. Use variables instead of inline arguments to ' +
        'overcome this limitation.',
    );
  }
}

export function storeKeyNameFromField(
  field: FieldNode,
  variables?: Object,
): string {
  let directivesObj: any = null;
  if (field.directives) {
    directivesObj = {};
    field.directives.forEach(directive => {
      directivesObj[directive.name.value] = {};

      if (directive.arguments) {
        directive.arguments.forEach(({ name, value }) =>
          valueToObjectRepresentation(
            directivesObj[directive.name.value],
            name,
            value,
            variables,
          ),
        );
      }
    });
  }

  let argObj: any = null;
  if (field.arguments && field.arguments.length) {
    argObj = {};
    field.arguments.forEach(({ name, value }) =>
      valueToObjectRepresentation(argObj, name, value, variables),
    );
  }

  return getStoreKeyName(field.name.value, argObj, directivesObj);
}

export type Directives = {
  [directiveName: string]: {
    [argName: string]: any;
  };
};

const KNOWN_DIRECTIVES: string[] = [
  'connection',
  'include',
  'skip',
  'client',
  'rest',
  'export',
];

export function getStoreKeyName(
  fieldName: string,
  args?: Object,
  directives?: Directives,
): string {
  if (
    directives &&
    directives['connection'] &&
    directives['connection']['key']
  ) {
    if (
      directives['connection']['filter'] &&
      (directives['connection']['filter'] as string[]).length > 0
    ) {
      const filterKeys = directives['connection']['filter']
        ? (directives['connection']['filter'] as string[])
        : [];
      filterKeys.sort();

      const queryArgs = args as { [key: string]: any };
      const filteredArgs = {} as { [key: string]: any };
      filterKeys.forEach(key => {
        filteredArgs[key] = queryArgs[key];
      });

      return `${directives['connection']['key']}(${JSON.stringify(
        filteredArgs,
      )})`;
    } else {
      return directives['connection']['key'];
    }
  }

  let completeFieldName: string = fieldName;

  if (args) {
    // We can't use `JSON.stringify` here since it's non-deterministic,
    // and can lead to different store key names being created even though
    // the `args` object used during creation has the same properties/values.
    const stringifiedArgs: string = stringify(args);
    completeFieldName += `(${stringifiedArgs})`;
  }

  if (directives) {
    Object.keys(directives).forEach(key => {
      if (KNOWN_DIRECTIVES.indexOf(key) !== -1) return;
      if (directives[key] && Object.keys(directives[key]).length) {
        completeFieldName += `@${key}(${JSON.stringify(directives[key])})`;
      } else {
        completeFieldName += `@${key}`;
      }
    });
  }

  return completeFieldName;
}

export function argumentsObjectFromField(
  field: FieldNode | DirectiveNode,
  variables: Object,
): Object {
  if (field.arguments && field.arguments.length) {
    const argObj: Object = {};
    field.arguments.forEach(({ name, value }) =>
      valueToObjectRepresentation(argObj, name, value, variables),
    );
    return argObj;
  }

  return null;
}

export function resultKeyNameFromField(field: FieldNode): string {
  return field.alias ? field.alias.value : field.name.value;
}

export function isField(selection: SelectionNode): selection is FieldNode {
  return selection.kind === 'Field';
}

export function isInlineFragment(
  selection: SelectionNode,
): selection is InlineFragmentNode {
  return selection.kind === 'InlineFragment';
}

export function isIdValue(idObject: StoreValue): idObject is IdValue {
  return idObject &&
    (idObject as IdValue | JsonValue).type === 'id' &&
    typeof (idObject as IdValue).generated === 'boolean';
}

export type IdConfig = {
  id: string;
  typename: string | undefined;
};

export function toIdValue(
  idConfig: string | IdConfig,
  generated = false,
): IdValue {
  return {
    type: 'id',
    generated,
    ...(typeof idConfig === 'string'
      ? { id: idConfig, typename: undefined }
      : idConfig),
  };
}

export function isJsonValue(jsonObject: StoreValue): jsonObject is JsonValue {
  return (
    jsonObject != null &&
    typeof jsonObject === 'object' &&
    (jsonObject as IdValue | JsonValue).type === 'json'
  );
}

function defaultValueFromVariable(node: VariableNode) {
  throw new InvariantError(`Variable nodes are not supported by valueFromNode`);
}

export type VariableValue = (node: VariableNode) => any;

/**
 * Evaluate a ValueNode and yield its value in its natural JS form.
 */
export function valueFromNode(
  node: ValueNode,
  onVariable: VariableValue = defaultValueFromVariable,
): any {
  switch (node.kind) {
    case 'Variable':
      return onVariable(node);
    case 'NullValue':
      return null;
    case 'IntValue':
      return parseInt(node.value, 10);
    case 'FloatValue':
      return parseFloat(node.value);
    case 'ListValue':
      return node.values.map(v => valueFromNode(v, onVariable));
    case 'ObjectValue': {
      const value: { [key: string]: any } = {};
      for (const field of node.fields) {
        value[field.name.value] = valueFromNode(field.value, onVariable);
      }
      return value;
    }
    default:
      return node.value;
  }
}
