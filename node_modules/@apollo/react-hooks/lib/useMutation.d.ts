import { OperationVariables } from '@apollo/react-common';
import { DocumentNode } from 'graphql';
import { MutationHookOptions, MutationTuple } from './types';
export declare function useMutation<TData = any, TVariables = OperationVariables>(mutation: DocumentNode, options?: MutationHookOptions<TData, TVariables>): MutationTuple<TData, TVariables>;
