"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var graphql_tag_1 = tslib_1.__importDefault(require("graphql-tag"));
var diffAgainstStore_1 = require("./diffAgainstStore");
var writeToStore_1 = require("./writeToStore");
var depTrackingCache_1 = require("../depTrackingCache");
var __1 = require("../");
var fragmentMatcherFunction = new __1.HeuristicFragmentMatcher().match;
function assertDeeplyFrozen(value, stack) {
    if (stack === void 0) { stack = []; }
    if (value !== null && typeof value === 'object' && stack.indexOf(value) < 0) {
        expect(Object.isExtensible(value)).toBe(false);
        expect(Object.isFrozen(value)).toBe(true);
        stack.push(value);
        Object.keys(value).forEach(function (key) {
            assertDeeplyFrozen(value[key], stack);
        });
        expect(stack.pop()).toBe(value);
    }
}
function storeRoundtrip(query, result, variables) {
    if (variables === void 0) { variables = {}; }
    var reader = new __1.StoreReader();
    var immutableReader = new __1.StoreReader({ freezeResults: true });
    var writer = new __1.StoreWriter();
    var store = writer.writeQueryToStore({
        result: result,
        query: query,
        variables: variables,
    });
    var readOptions = {
        store: store,
        query: query,
        variables: variables,
        fragmentMatcherFunction: fragmentMatcherFunction,
    };
    var reconstructedResult = reader.readQueryFromStore(readOptions);
    expect(reconstructedResult).toEqual(result);
    expect(store).toBeInstanceOf(depTrackingCache_1.DepTrackingCache);
    expect(reader.readQueryFromStore(readOptions)).toBe(reconstructedResult);
    var immutableResult = immutableReader.readQueryFromStore(readOptions);
    expect(immutableResult).toEqual(reconstructedResult);
    expect(immutableReader.readQueryFromStore(readOptions)).toBe(immutableResult);
    if (process.env.NODE_ENV !== 'production') {
        try {
            immutableResult.illegal = 'this should not work';
            throw new Error('unreached');
        }
        catch (e) {
            expect(e.message).not.toMatch(/unreached/);
            expect(e).toBeInstanceOf(TypeError);
        }
        assertDeeplyFrozen(immutableResult);
    }
    writer.writeQueryToStore({
        store: store,
        result: { oyez: 1234 },
        query: graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n      {\n        oyez\n      }\n    "], ["\n      {\n        oyez\n      }\n    "]))),
    });
    var deletedRootResult = reader.readQueryFromStore(readOptions);
    expect(deletedRootResult).toEqual(result);
    if (deletedRootResult === reconstructedResult) {
        return;
    }
    Object.keys(result).forEach(function (key) {
        expect(deletedRootResult[key]).toBe(reconstructedResult[key]);
    });
}
describe('roundtrip', function () {
    it('real graphql result', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_2 || (templateObject_2 = tslib_1.__makeTemplateObject(["\n        {\n          people_one(id: \"1\") {\n            name\n          }\n        }\n      "], ["\n        {\n          people_one(id: \"1\") {\n            name\n          }\n        }\n      "]))), {
            people_one: {
                name: 'Luke Skywalker',
            },
        });
    });
    it('multidimensional array (#776)', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_3 || (templateObject_3 = tslib_1.__makeTemplateObject(["\n        {\n          rows {\n            value\n          }\n        }\n      "], ["\n        {\n          rows {\n            value\n          }\n        }\n      "]))), {
            rows: [[{ value: 1 }, { value: 2 }], [{ value: 3 }, { value: 4 }]],
        });
    });
    it('array with null values (#1551)', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_4 || (templateObject_4 = tslib_1.__makeTemplateObject(["\n        {\n          list {\n            value\n          }\n        }\n      "], ["\n        {\n          list {\n            value\n          }\n        }\n      "]))), {
            list: [null, { value: 1 }],
        });
    });
    it('enum arguments', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_5 || (templateObject_5 = tslib_1.__makeTemplateObject(["\n        {\n          hero(episode: JEDI) {\n            name\n          }\n        }\n      "], ["\n        {\n          hero(episode: JEDI) {\n            name\n          }\n        }\n      "]))), {
            hero: {
                name: 'Luke Skywalker',
            },
        });
    });
    it('with an alias', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_6 || (templateObject_6 = tslib_1.__makeTemplateObject(["\n        {\n          luke: people_one(id: \"1\") {\n            name\n          }\n          vader: people_one(id: \"4\") {\n            name\n          }\n        }\n      "], ["\n        {\n          luke: people_one(id: \"1\") {\n            name\n          }\n          vader: people_one(id: \"4\") {\n            name\n          }\n        }\n      "]))), {
            luke: {
                name: 'Luke Skywalker',
            },
            vader: {
                name: 'Darth Vader',
            },
        });
    });
    it('with variables', function () {
        storeRoundtrip(graphql_tag_1.default(templateObject_7 || (templateObject_7 = tslib_1.__makeTemplateObject(["\n        {\n          luke: people_one(id: $lukeId) {\n            name\n          }\n          vader: people_one(id: $vaderId) {\n            name\n          }\n        }\n      "], ["\n        {\n          luke: people_one(id: $lukeId) {\n            name\n          }\n          vader: people_one(id: $vaderId) {\n            name\n          }\n        }\n      "]))), {
            luke: {
                name: 'Luke Skywalker',
            },
            vader: {
                name: 'Darth Vader',
            },
        }, {
            lukeId: '1',
            vaderId: '4',
        });
    });
    it('with GraphQLJSON scalar type', function () {
        var updateClub = {
            uid: '1d7f836018fc11e68d809dfee940f657',
            name: 'Eple',
            settings: {
                name: 'eple',
                currency: 'AFN',
                calendarStretch: 2,
                defaultPreAllocationPeriod: 1,
                confirmationEmailCopy: null,
                emailDomains: null,
            },
        };
        storeRoundtrip(graphql_tag_1.default(templateObject_8 || (templateObject_8 = tslib_1.__makeTemplateObject(["\n        {\n          updateClub {\n            uid\n            name\n            settings\n          }\n        }\n      "], ["\n        {\n          updateClub {\n            uid\n            name\n            settings\n          }\n        }\n      "]))), {
            updateClub: updateClub,
        });
        expect(Object.isExtensible(updateClub)).toBe(true);
        expect(Object.isFrozen(updateClub)).toBe(false);
    });
    describe('directives', function () {
        it('should be able to query with skip directive true', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_9 || (templateObject_9 = tslib_1.__makeTemplateObject(["\n          query {\n            fortuneCookie @skip(if: true)\n          }\n        "], ["\n          query {\n            fortuneCookie @skip(if: true)\n          }\n        "]))), {});
        });
        it('should be able to query with skip directive false', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_10 || (templateObject_10 = tslib_1.__makeTemplateObject(["\n          query {\n            fortuneCookie @skip(if: false)\n          }\n        "], ["\n          query {\n            fortuneCookie @skip(if: false)\n          }\n        "]))), { fortuneCookie: 'live long and prosper' });
        });
    });
    describe('fragments', function () {
        it('should work on null fields', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_11 || (templateObject_11 = tslib_1.__makeTemplateObject(["\n          query {\n            field {\n              ... on Obj {\n                stuff\n              }\n            }\n          }\n        "], ["\n          query {\n            field {\n              ... on Obj {\n                stuff\n              }\n            }\n          }\n        "]))), {
                field: null,
            });
        });
        it('should work on basic inline fragments', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_12 || (templateObject_12 = tslib_1.__makeTemplateObject(["\n          query {\n            field {\n              __typename\n              ... on Obj {\n                stuff\n              }\n            }\n          }\n        "], ["\n          query {\n            field {\n              __typename\n              ... on Obj {\n                stuff\n              }\n            }\n          }\n        "]))), {
                field: {
                    __typename: 'Obj',
                    stuff: 'Result',
                },
            });
        });
        it('should resolve on union types with inline fragments without typenames with warning', function () {
            return writeToStore_1.withWarning(function () {
                storeRoundtrip(graphql_tag_1.default(templateObject_13 || (templateObject_13 = tslib_1.__makeTemplateObject(["\n            query {\n              all_people {\n                name\n                ... on Jedi {\n                  side\n                }\n                ... on Droid {\n                  model\n                }\n              }\n            }\n          "], ["\n            query {\n              all_people {\n                name\n                ... on Jedi {\n                  side\n                }\n                ... on Droid {\n                  model\n                }\n              }\n            }\n          "]))), {
                    all_people: [
                        {
                            name: 'Luke Skywalker',
                            side: 'bright',
                        },
                        {
                            name: 'R2D2',
                            model: 'astromech',
                        },
                    ],
                });
            }, /using fragments/);
        });
        it('should throw an error on two of the same inline fragment types', function () {
            return expect(function () {
                storeRoundtrip(graphql_tag_1.default(templateObject_14 || (templateObject_14 = tslib_1.__makeTemplateObject(["\n            query {\n              all_people {\n                __typename\n                name\n                ... on Jedi {\n                  side\n                }\n                ... on Jedi {\n                  rank\n                }\n              }\n            }\n          "], ["\n            query {\n              all_people {\n                __typename\n                name\n                ... on Jedi {\n                  side\n                }\n                ... on Jedi {\n                  rank\n                }\n              }\n            }\n          "]))), {
                    all_people: [
                        {
                            __typename: 'Jedi',
                            name: 'Luke Skywalker',
                            side: 'bright',
                        },
                    ],
                });
            }).toThrowError(/Can\'t find field rank on object/);
        });
        it('should resolve fields it can on interface with non matching inline fragments', function () {
            return diffAgainstStore_1.withError(function () {
                storeRoundtrip(graphql_tag_1.default(templateObject_15 || (templateObject_15 = tslib_1.__makeTemplateObject(["\n            query {\n              dark_forces {\n                __typename\n                name\n                ... on Droid {\n                  model\n                }\n              }\n            }\n          "], ["\n            query {\n              dark_forces {\n                __typename\n                name\n                ... on Droid {\n                  model\n                }\n              }\n            }\n          "]))), {
                    dark_forces: [
                        {
                            __typename: 'Droid',
                            name: '8t88',
                            model: '88',
                        },
                        {
                            __typename: 'Darth',
                            name: 'Anakin Skywalker',
                        },
                    ],
                });
            }, /IntrospectionFragmentMatcher/);
        });
        it('should resolve on union types with spread fragments', function () {
            return diffAgainstStore_1.withError(function () {
                storeRoundtrip(graphql_tag_1.default(templateObject_16 || (templateObject_16 = tslib_1.__makeTemplateObject(["\n            fragment jediFragment on Jedi {\n              side\n            }\n\n            fragment droidFragment on Droid {\n              model\n            }\n\n            query {\n              all_people {\n                __typename\n                name\n                ...jediFragment\n                ...droidFragment\n              }\n            }\n          "], ["\n            fragment jediFragment on Jedi {\n              side\n            }\n\n            fragment droidFragment on Droid {\n              model\n            }\n\n            query {\n              all_people {\n                __typename\n                name\n                ...jediFragment\n                ...droidFragment\n              }\n            }\n          "]))), {
                    all_people: [
                        {
                            __typename: 'Jedi',
                            name: 'Luke Skywalker',
                            side: 'bright',
                        },
                        {
                            __typename: 'Droid',
                            name: 'R2D2',
                            model: 'astromech',
                        },
                    ],
                });
            }, /IntrospectionFragmentMatcher/);
        });
        it('should work with a fragment on the actual interface or union', function () {
            return diffAgainstStore_1.withError(function () {
                storeRoundtrip(graphql_tag_1.default(templateObject_17 || (templateObject_17 = tslib_1.__makeTemplateObject(["\n            fragment jediFragment on Character {\n              side\n            }\n\n            fragment droidFragment on Droid {\n              model\n            }\n\n            query {\n              all_people {\n                name\n                __typename\n                ...jediFragment\n                ...droidFragment\n              }\n            }\n          "], ["\n            fragment jediFragment on Character {\n              side\n            }\n\n            fragment droidFragment on Droid {\n              model\n            }\n\n            query {\n              all_people {\n                name\n                __typename\n                ...jediFragment\n                ...droidFragment\n              }\n            }\n          "]))), {
                    all_people: [
                        {
                            __typename: 'Jedi',
                            name: 'Luke Skywalker',
                            side: 'bright',
                        },
                        {
                            __typename: 'Droid',
                            name: 'R2D2',
                            model: 'astromech',
                        },
                    ],
                });
            }, /IntrospectionFragmentMatcher/);
        });
        it('should throw on error on two of the same spread fragment types', function () {
            expect(function () {
                return storeRoundtrip(graphql_tag_1.default(templateObject_18 || (templateObject_18 = tslib_1.__makeTemplateObject(["\n            fragment jediSide on Jedi {\n              side\n            }\n\n            fragment jediRank on Jedi {\n              rank\n            }\n\n            query {\n              all_people {\n                __typename\n                name\n                ...jediSide\n                ...jediRank\n              }\n            }\n          "], ["\n            fragment jediSide on Jedi {\n              side\n            }\n\n            fragment jediRank on Jedi {\n              rank\n            }\n\n            query {\n              all_people {\n                __typename\n                name\n                ...jediSide\n                ...jediRank\n              }\n            }\n          "]))), {
                    all_people: [
                        {
                            __typename: 'Jedi',
                            name: 'Luke Skywalker',
                            side: 'bright',
                        },
                    ],
                });
            }).toThrowError(/Can\'t find field rank on object/);
        });
        it('should resolve on @include and @skip with inline fragments', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_19 || (templateObject_19 = tslib_1.__makeTemplateObject(["\n          query {\n            person {\n              name\n              __typename\n              ... on Jedi @include(if: true) {\n                side\n              }\n              ... on Droid @skip(if: true) {\n                model\n              }\n            }\n          }\n        "], ["\n          query {\n            person {\n              name\n              __typename\n              ... on Jedi @include(if: true) {\n                side\n              }\n              ... on Droid @skip(if: true) {\n                model\n              }\n            }\n          }\n        "]))), {
                person: {
                    __typename: 'Jedi',
                    name: 'Luke Skywalker',
                    side: 'bright',
                },
            });
        });
        it('should resolve on @include and @skip with spread fragments', function () {
            storeRoundtrip(graphql_tag_1.default(templateObject_20 || (templateObject_20 = tslib_1.__makeTemplateObject(["\n          fragment jediFragment on Jedi {\n            side\n          }\n\n          fragment droidFragment on Droid {\n            model\n          }\n\n          query {\n            person {\n              name\n              __typename\n              ...jediFragment @include(if: true)\n              ...droidFragment @skip(if: true)\n            }\n          }\n        "], ["\n          fragment jediFragment on Jedi {\n            side\n          }\n\n          fragment droidFragment on Droid {\n            model\n          }\n\n          query {\n            person {\n              name\n              __typename\n              ...jediFragment @include(if: true)\n              ...droidFragment @skip(if: true)\n            }\n          }\n        "]))), {
                person: {
                    __typename: 'Jedi',
                    name: 'Luke Skywalker',
                    side: 'bright',
                },
            });
        });
    });
});
var templateObject_1, templateObject_2, templateObject_3, templateObject_4, templateObject_5, templateObject_6, templateObject_7, templateObject_8, templateObject_9, templateObject_10, templateObject_11, templateObject_12, templateObject_13, templateObject_14, templateObject_15, templateObject_16, templateObject_17, templateObject_18, templateObject_19, templateObject_20;
//# sourceMappingURL=roundtrip.js.map