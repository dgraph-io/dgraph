/// <reference types="node" />
export declare class InvariantError extends Error {
    framesToPop: number;
    name: string;
    constructor(message?: string | number);
}
export declare function invariant(condition: any, message?: string | number): void;
export declare namespace invariant {
    const warn: (...args: any[]) => void;
    const error: (...args: any[]) => void;
}
declare let processStub: NodeJS.Process;
export { processStub as process };
export default invariant;
