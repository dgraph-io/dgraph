"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var query = {
    kind: 'Document',
    definitions: [
        {
            kind: 'OperationDefinition',
            operation: 'query',
            name: null,
            variableDefinitions: null,
            directives: [],
            selectionSet: {
                kind: 'SelectionSet',
                selections: [
                    {
                        kind: 'Field',
                        alias: null,
                        name: {
                            kind: 'Name',
                            value: '__schema',
                        },
                        arguments: [],
                        directives: [],
                        selectionSet: {
                            kind: 'SelectionSet',
                            selections: [
                                {
                                    kind: 'Field',
                                    alias: null,
                                    name: {
                                        kind: 'Name',
                                        value: 'types',
                                    },
                                    arguments: [],
                                    directives: [],
                                    selectionSet: {
                                        kind: 'SelectionSet',
                                        selections: [
                                            {
                                                kind: 'Field',
                                                alias: null,
                                                name: {
                                                    kind: 'Name',
                                                    value: 'kind',
                                                },
                                                arguments: [],
                                                directives: [],
                                                selectionSet: null,
                                            },
                                            {
                                                kind: 'Field',
                                                alias: null,
                                                name: {
                                                    kind: 'Name',
                                                    value: 'name',
                                                },
                                                arguments: [],
                                                directives: [],
                                                selectionSet: null,
                                            },
                                            {
                                                kind: 'Field',
                                                alias: null,
                                                name: {
                                                    kind: 'Name',
                                                    value: 'possibleTypes',
                                                },
                                                arguments: [],
                                                directives: [],
                                                selectionSet: {
                                                    kind: 'SelectionSet',
                                                    selections: [
                                                        {
                                                            kind: 'Field',
                                                            alias: null,
                                                            name: {
                                                                kind: 'Name',
                                                                value: 'name',
                                                            },
                                                            arguments: [],
                                                            directives: [],
                                                            selectionSet: null,
                                                        },
                                                    ],
                                                },
                                            },
                                        ],
                                    },
                                },
                            ],
                        },
                    },
                ],
            },
        },
    ],
};
exports.default = query;
//# sourceMappingURL=fragmentMatcherIntrospectionQuery.js.map