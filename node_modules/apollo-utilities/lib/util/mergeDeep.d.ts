export declare type TupleToIntersection<T extends any[]> = T extends [infer A] ? A : T extends [infer A, infer B] ? A & B : T extends [infer A, infer B, infer C] ? A & B & C : T extends [infer A, infer B, infer C, infer D] ? A & B & C & D : T extends [infer A, infer B, infer C, infer D, infer E] ? A & B & C & D & E : T extends (infer U)[] ? U : any;
export declare function mergeDeep<T extends any[]>(...sources: T): TupleToIntersection<T>;
export declare function mergeDeepArray<T>(sources: T[]): T;
//# sourceMappingURL=mergeDeep.d.ts.map