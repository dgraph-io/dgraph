+++
title = "Dgraph Schema Fragment"
weight = 8
[menu.main]
    parent = "schema"
+++

While editing your schema, you might find it useful to include this GraphQL schema fragment.  It sets up the definitions of the directives, etc. (like `@search`) that you'll use in your schema.  If your editor is GraphQL aware, it may give you errors if you don't have this available and context sensitive help if you do.

Don't include it in your input schema to Dgraph - use your editing environment to set it up as an import.  The details will depend on your setup.

```graphql
scalar DateTime

enum DgraphIndex {
	int
	float
	bool
	hash
	exact
	term
	fulltext
	trigram
	regexp
	year
	month
	day
	hour
}

input AuthRule {
	and: [AuthRule]
	or: [AuthRule]
	not: AuthRule
	rule: String
}

enum HTTPMethod {
	GET
	POST
	PUT
	PATCH
	DELETE
}

enum Mode {
	BATCH
	SINGLE
}

input CustomHTTP {
	url: String!
	method: HTTPMethod!
	body: String
	graphql: String
	mode: Mode
	forwardHeaders: [String!]
	secretHeaders: [String!]
	introspectionHeaders: [String!]
	skipIntrospection: Boolean
}

directive @hasInverse(field: String!) on FIELD_DEFINITION
directive @search(by: [DgraphIndex!]) on FIELD_DEFINITION
directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION
directive @id on FIELD_DEFINITION
directive @withSubscription on OBJECT | INTERFACE
directive @secret(field: String!, pred: String) on OBJECT | INTERFACE
directive @auth(
	query: AuthRule,
	add: AuthRule,
	update: AuthRule,
	delete: AuthRule) on OBJECT
directive @custom(http: CustomHTTP) on FIELD_DEFINITION
directive @remote on OBJECT | INTERFACE
```
