"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var fragmentMatcher_1 = require("../fragmentMatcher");
var objectCache_1 = require("../objectCache");
var inMemoryCache_1 = require("../inMemoryCache");
var graphql_tag_1 = tslib_1.__importDefault(require("graphql-tag"));
describe('FragmentMatcher', function () {
    it('can match against the root Query', function () {
        var cache = new inMemoryCache_1.InMemoryCache({
            addTypename: true,
        });
        var query = graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n      query AllPeople {\n        people {\n          id\n          name\n        }\n        ...PeopleTypes\n      }\n      fragment PeopleTypes on Query {\n        __type(name: \"Person\") {\n          name\n          kind\n        }\n      }\n    "], ["\n      query AllPeople {\n        people {\n          id\n          name\n        }\n        ...PeopleTypes\n      }\n      fragment PeopleTypes on Query {\n        __type(name: \"Person\") {\n          name\n          kind\n        }\n      }\n    "])));
        var data = {
            people: [
                {
                    __typename: 'Person',
                    id: 123,
                    name: 'Ben',
                },
            ],
            __type: {
                __typename: '__Type',
                name: 'Person',
                kind: 'OBJECT',
            },
        };
        cache.writeQuery({ query: query, data: data });
        expect(cache.readQuery({ query: query })).toEqual(data);
    });
});
describe('IntrospectionFragmentMatcher', function () {
    it('will throw an error if match is called if it is not ready', function () {
        var ifm = new fragmentMatcher_1.IntrospectionFragmentMatcher();
        expect(function () { return ifm.match(); }).toThrowError(/called before/);
    });
    it('can be seeded with an introspection query result', function () {
        var ifm = new fragmentMatcher_1.IntrospectionFragmentMatcher({
            introspectionQueryResultData: {
                __schema: {
                    types: [
                        {
                            kind: 'UNION',
                            name: 'Item',
                            possibleTypes: [
                                {
                                    name: 'ItemA',
                                },
                                {
                                    name: 'ItemB',
                                },
                            ],
                        },
                    ],
                },
            },
        });
        var store = objectCache_1.defaultNormalizedCacheFactory({
            a: {
                __typename: 'ItemB',
            },
        });
        var idValue = {
            type: 'id',
            id: 'a',
            generated: false,
        };
        var readStoreContext = {
            store: store,
            returnPartialData: false,
            hasMissingField: false,
            cacheRedirects: {},
        };
        expect(ifm.match(idValue, 'Item', readStoreContext)).toBe(true);
        expect(ifm.match(idValue, 'NotAnItem', readStoreContext)).toBe(false);
    });
});
var templateObject_1;
//# sourceMappingURL=fragmentMatcher.js.map