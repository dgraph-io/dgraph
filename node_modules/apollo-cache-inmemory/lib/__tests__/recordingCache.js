"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var inMemoryCache_1 = require("../inMemoryCache");
var objectCache_1 = require("../objectCache");
describe('OptimisticCacheLayer', function () {
    function makeLayer(root) {
        return new inMemoryCache_1.OptimisticCacheLayer('whatever', root, function () { });
    }
    describe('returns correct values during recording', function () {
        var data = {
            Human: { __typename: 'Human', name: 'Mark' },
            Animal: { __typename: 'Mouse', name: '🐭' },
        };
        var dataToRecord = {
            Human: { __typename: 'Human', name: 'John' },
        };
        var underlyingCache = new objectCache_1.ObjectCache(data);
        var cache = makeLayer(underlyingCache);
        beforeEach(function () {
            cache = makeLayer(underlyingCache);
        });
        it('should passthrough values if not defined in recording', function () {
            expect(cache.get('Human')).toBe(data.Human);
            expect(cache.get('Animal')).toBe(data.Animal);
        });
        it('should return values defined during recording', function () {
            cache.set('Human', dataToRecord.Human);
            expect(cache.get('Human')).toBe(dataToRecord.Human);
            expect(underlyingCache.get('Human')).toBe(data.Human);
        });
        it('should return undefined for values deleted during recording', function () {
            expect(cache.get('Animal')).toBe(data.Animal);
            cache.delete('Animal');
            expect(cache.get('Animal')).toBeUndefined();
            expect(cache.toObject()).toHaveProperty('Animal');
            expect(underlyingCache.get('Animal')).toBe(data.Animal);
        });
    });
    describe('returns correct result of a recorded transaction', function () {
        var data = {
            Human: { __typename: 'Human', name: 'Mark' },
            Animal: { __typename: 'Mouse', name: '🐭' },
        };
        var dataToRecord = {
            Human: { __typename: 'Human', name: 'John' },
        };
        var underlyingCache = new objectCache_1.ObjectCache(data);
        var cache = makeLayer(underlyingCache);
        var recording;
        beforeEach(function () {
            cache = makeLayer(underlyingCache);
            cache.set('Human', dataToRecord.Human);
            cache.delete('Animal');
            recording = cache.toObject();
        });
        it('should contain the property indicating deletion', function () {
            expect(recording).toHaveProperty('Animal');
        });
        it('should have recorded the changes made during recording', function () {
            expect(recording).toEqual({
                Human: dataToRecord.Human,
                Animal: undefined,
            });
        });
        it('should keep the original data unaffected', function () {
            expect(underlyingCache.toObject()).toEqual(data);
        });
    });
});
//# sourceMappingURL=recordingCache.js.map