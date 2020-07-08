"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var objectCache_1 = require("../objectCache");
describe('ObjectCache', function () {
    it('should create an empty cache', function () {
        var cache = new objectCache_1.ObjectCache();
        expect(cache.toObject()).toEqual({});
    });
    it('should create a cache based on an Object', function () {
        var contents = { a: {} };
        var cache = new objectCache_1.ObjectCache(contents);
        expect(cache.toObject()).toEqual(contents);
    });
    it("should .get() an object from the store by dataId", function () {
        var contents = { a: {} };
        var cache = new objectCache_1.ObjectCache(contents);
        expect(cache.get('a')).toBe(contents.a);
    });
    it("should .set() an object from the store by dataId", function () {
        var obj = {};
        var cache = new objectCache_1.ObjectCache();
        cache.set('a', obj);
        expect(cache.get('a')).toBe(obj);
    });
    it("should .clear() the store", function () {
        var obj = {};
        var cache = new objectCache_1.ObjectCache();
        cache.set('a', obj);
        cache.clear();
        expect(cache.get('a')).toBeUndefined();
    });
});
//# sourceMappingURL=objectCache.js.map