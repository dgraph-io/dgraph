import { DocumentNode, DirectiveNode, FragmentDefinitionNode, ArgumentNode, FragmentSpreadNode, VariableDefinitionNode } from 'graphql';
export declare type RemoveNodeConfig<N> = {
    name?: string;
    test?: (node: N) => boolean;
    remove?: boolean;
};
export declare type GetNodeConfig<N> = {
    name?: string;
    test?: (node: N) => boolean;
};
export declare type RemoveDirectiveConfig = RemoveNodeConfig<DirectiveNode>;
export declare type GetDirectiveConfig = GetNodeConfig<DirectiveNode>;
export declare type RemoveArgumentsConfig = RemoveNodeConfig<ArgumentNode>;
export declare type GetFragmentSpreadConfig = GetNodeConfig<FragmentSpreadNode>;
export declare type RemoveFragmentSpreadConfig = RemoveNodeConfig<FragmentSpreadNode>;
export declare type RemoveFragmentDefinitionConfig = RemoveNodeConfig<FragmentDefinitionNode>;
export declare type RemoveVariableDefinitionConfig = RemoveNodeConfig<VariableDefinitionNode>;
export declare function removeDirectivesFromDocument(directives: RemoveDirectiveConfig[], doc: DocumentNode): DocumentNode | null;
export declare function addTypenameToDocument(doc: DocumentNode): DocumentNode;
export declare function removeConnectionDirectiveFromDocument(doc: DocumentNode): DocumentNode;
export declare function getDirectivesFromDocument(directives: GetDirectiveConfig[], doc: DocumentNode): DocumentNode;
export declare function removeArgumentsFromDocument(config: RemoveArgumentsConfig[], doc: DocumentNode): DocumentNode;
export declare function removeFragmentSpreadFromDocument(config: RemoveFragmentSpreadConfig[], doc: DocumentNode): DocumentNode;
export declare function buildQueryFromSelectionSet(document: DocumentNode): DocumentNode;
export declare function removeClientSetsFromDocument(document: DocumentNode): DocumentNode | null;
//# sourceMappingURL=transform.d.ts.map