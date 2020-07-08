import { ApolloLink } from '../link';
export declare function checkCalls<T>(calls: any[], results: Array<T>): void;
export interface TestResultType {
    link: ApolloLink;
    results?: any[];
    query?: string;
    done?: () => void;
    context?: any;
    variables?: any;
}
export declare function testLinkResults(params: TestResultType): void;
//# sourceMappingURL=testingUtils.d.ts.map