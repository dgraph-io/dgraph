import { DocumentNode } from 'graphql';
export declare namespace DataProxy {
    interface Query<TVariables> {
        query: DocumentNode;
        variables?: TVariables;
    }
    interface Fragment<TVariables> {
        id: string;
        fragment: DocumentNode;
        fragmentName?: string;
        variables?: TVariables;
    }
    interface WriteQueryOptions<TData, TVariables> extends Query<TVariables> {
        data: TData;
    }
    interface WriteFragmentOptions<TData, TVariables> extends Fragment<TVariables> {
        data: TData;
    }
    interface WriteDataOptions<TData> {
        data: TData;
        id?: string;
    }
    type DiffResult<T> = {
        result?: T;
        complete?: boolean;
    };
}
export interface DataProxy {
    readQuery<QueryType, TVariables = any>(options: DataProxy.Query<TVariables>, optimistic?: boolean): QueryType | null;
    readFragment<FragmentType, TVariables = any>(options: DataProxy.Fragment<TVariables>, optimistic?: boolean): FragmentType | null;
    writeQuery<TData = any, TVariables = any>(options: DataProxy.WriteQueryOptions<TData, TVariables>): void;
    writeFragment<TData = any, TVariables = any>(options: DataProxy.WriteFragmentOptions<TData, TVariables>): void;
    writeData<TData = any>(options: DataProxy.WriteDataOptions<TData>): void;
}
//# sourceMappingURL=DataProxy.d.ts.map