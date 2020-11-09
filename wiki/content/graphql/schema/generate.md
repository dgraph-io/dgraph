+++
title = "The @generate directive"
weight = 6
[menu.main]
    parent = "schema"
    identifier = "schema-generate"
+++

The `@generate` directive is used to specify GraphQL APIs to generate for a given type.

Here's the GraphQL definition of the directive
```graphql
input GenerateQueryParams {
	get: Boolean
	query: Boolean
	password: Boolean
	aggregate: Boolean
}

input GenerateMutationParams {
	add: Boolean
	update: Boolean
	delete: Boolean
}
directive @generate(
	query: GenerateQueryParams,
	mutation: GenerateMutationParams,
	subscription: Boolean) on OBJECT | INTERFACE

```

By passing `true` to the `Boolean` variables inside `@generate` directive, the corresponding APIs are generated while passing `false` forbids the generation of corresponding APIs. The default variable of `subscription` variable is `false` while the default value of
other variables is `true`. Therefore, if no `@generate` directive is specified for a type, all queries and mutations except subscription are generated.

## Example of @generate directive

```graphql
type Post @withSubscription @generate(
    query: {
        get: false,
        query: true,
        aggregate: false
    },
    mutation: {
        add: true,
        delete: false
    },
    subscription: false
) {
    id: ID!
    name: String!
}
```

The above GraphQL schema will generate `queryPerson` query and `addPerson`, `updatePerson` mutation. It won't generate `getPerson`, `aggregatePerson` query and `deletePerson` mutation as these have been marked as `false` using the @generate directive.
Note that the `updatePerson` is generated because the default value of `update` variable is `true`.
