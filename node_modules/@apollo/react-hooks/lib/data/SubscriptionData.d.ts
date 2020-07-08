import { ApolloContextValue, SubscriptionResult } from '@apollo/react-common';
import { OperationData } from './OperationData';
import { SubscriptionOptions } from '../types';
export declare class SubscriptionData<TData = any, TVariables = any> extends OperationData<SubscriptionOptions<TData, TVariables>> {
    private setResult;
    private currentObservable;
    constructor({ options, context, setResult }: {
        options: SubscriptionOptions<TData, TVariables>;
        context: ApolloContextValue;
        setResult: any;
    });
    execute(result: SubscriptionResult<TData>): {
        variables: TVariables | undefined;
        loading: boolean;
        data?: TData | undefined;
        error?: import("apollo-client").ApolloError | undefined;
    };
    afterExecute(): void;
    cleanup(): void;
    private initialize;
    private startSubscription;
    private getLoadingResult;
    private updateResult;
    private updateCurrentData;
    private updateError;
    private completeSubscription;
    private endSubscription;
}
