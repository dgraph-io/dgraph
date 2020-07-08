jest.mock('../objectCache', function () {
    var _a = require('../mapCache'), MapCache = _a.MapCache, mapNormalizedCacheFactory = _a.mapNormalizedCacheFactory;
    return {
        ObjectCache: MapCache,
        defaultNormalizedCacheFactory: mapNormalizedCacheFactory,
    };
});
describe('MapCache', function () {
    require('./objectCache');
    require('./cache');
    require('./diffAgainstStore');
    require('./fragmentMatcher');
    require('./readFromStore');
    require('./diffAgainstStore');
    require('./roundtrip');
    require('./writeToStore');
});
//# sourceMappingURL=mapCache.js.map