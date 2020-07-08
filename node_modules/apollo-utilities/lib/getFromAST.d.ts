import { DocumentNode, OperationDefinitionNode, FragmentDefinitionNode } from 'graphql';
import { JsonValue } from './storeUtils';
export declare function getMutationDefinition(doc: DocumentNode): OperationDefinitionNode;
export declare function checkDocument(doc: DocumentNode): DocumentNode;
export declare function getOperationDefinition(doc: DocumentNode): OperationDefinitionNode | undefined;
export declare function getOperationDefinitionOrDie(document: DocumentNode): OperationDefinitionNode;
export declare function getOperationName(doc: DocumentNode): string | null;
export declare function getFragmentDefinitions(doc: DocumentNode): FragmentDefinitionNode[];
export declare function getQueryDefinition(doc: DocumentNode): OperationDefinitionNode;
export declare function getFragmentDefinition(doc: DocumentNode): FragmentDefinitionNode;
export declare function getMainDefinition(queryDoc: DocumentNode): OperationDefinitionNode | FragmentDefinitionNode;
export interface FragmentMap {
    [fragmentName: string]: FragmentDefinitionNode;
}
export declare function createFragmentMap(fragments?: FragmentDefinitionNode[]): FragmentMap;
export declare function getDefaultValues(definition: OperationDefinitionNode | undefined): {
    [key: string]: JsonValue;
};
export declare function variablesInOperation(operation: OperationDefinitionNode): Set<string>;
//# sourceMappingURL=getFromAST.d.ts.map