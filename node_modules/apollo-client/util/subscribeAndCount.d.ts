/// <reference types="jest" />
import { ObservableQuery } from '../core/ObservableQuery';
import { ApolloQueryResult } from '../core/types';
import { Subscription } from '../util/Observable';
export default function subscribeAndCount(done: jest.DoneCallback, observable: ObservableQuery<any>, cb: (handleCount: number, result: ApolloQueryResult<any>) => any): Subscription;
//# sourceMappingURL=subscribeAndCount.d.ts.map