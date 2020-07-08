"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
function getEnv() {
    if (typeof process !== 'undefined' && process.env.NODE_ENV) {
        return process.env.NODE_ENV;
    }
    return 'development';
}
exports.getEnv = getEnv;
function isEnv(env) {
    return getEnv() === env;
}
exports.isEnv = isEnv;
function isProduction() {
    return isEnv('production') === true;
}
exports.isProduction = isProduction;
function isDevelopment() {
    return isEnv('development') === true;
}
exports.isDevelopment = isDevelopment;
function isTest() {
    return isEnv('test') === true;
}
exports.isTest = isTest;
//# sourceMappingURL=environment.js.map