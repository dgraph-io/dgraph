"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var inMemoryCache_1 = require("./inMemoryCache");
exports.InMemoryCache = inMemoryCache_1.InMemoryCache;
exports.defaultDataIdFromObject = inMemoryCache_1.defaultDataIdFromObject;
tslib_1.__exportStar(require("./readFromStore"), exports);
tslib_1.__exportStar(require("./writeToStore"), exports);
tslib_1.__exportStar(require("./fragmentMatcher"), exports);
tslib_1.__exportStar(require("./objectCache"), exports);
//# sourceMappingURL=index.js.map