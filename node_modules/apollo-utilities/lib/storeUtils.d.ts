import { DirectiveNode, FieldNode, IntValueNode, FloatValueNode, StringValueNode, BooleanValueNode, EnumValueNode, VariableNode, InlineFragmentNode, ValueNode, SelectionNode, NameNode } from 'graphql';
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
export declare type ListValue = Array<null | IdValue>;
export declare type StoreValue = number | string | string[] | IdValue | ListValue | JsonValue | null | undefined | void | Object;
export declare type ScalarValue = StringValueNode | BooleanValueNode | EnumValueNode;
export declare function isScalarValue(value: ValueNode): value is ScalarValue;
export declare type NumberValue = IntValueNode | FloatValueNode;
export declare function isNumberValue(value: ValueNode): value is NumberValue;
export declare function valueToObjectRepresentation(argObj: any, name: NameNode, value: ValueNode, variables?: Object): void;
export declare function storeKeyNameFromField(field: FieldNode, variables?: Object): string;
export declare type Directives = {
    [directiveName: string]: {
        [argName: string]: any;
    };
};
export declare function getStoreKeyName(fieldName: string, args?: Object, directives?: Directives): string;
export declare function argumentsObjectFromField(field: FieldNode | DirectiveNode, variables: Object): Object;
export declare function resultKeyNameFromField(field: FieldNode): string;
export declare function isField(selection: SelectionNode): selection is FieldNode;
export declare function isInlineFragment(selection: SelectionNode): selection is InlineFragmentNode;
export declare function isIdValue(idObject: StoreValue): idObject is IdValue;
export declare type IdConfig = {
    id: string;
    typename: string | undefined;
};
export declare function toIdValue(idConfig: string | IdConfig, generated?: boolean): IdValue;
export declare function isJsonValue(jsonObject: StoreValue): jsonObject is JsonValue;
export declare type VariableValue = (node: VariableNode) => any;
export declare function valueFromNode(node: ValueNode, onVariable?: VariableValue): any;
//# sourceMappingURL=storeUtils.d.ts.map