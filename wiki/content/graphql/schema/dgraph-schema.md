+++
title = "Dgraph Schema Fragment"
weight = 9
[menu.main]
    parent = "schema"
+++

While editing your schema, you might find it useful to include this GraphQL schema fragment.  It sets up the definitions of the directives, etc. (like `@search`) that you'll use in your schema.  If your editor is GraphQL aware, it may give you errors if you don't have this available and context sensitive help if you do.

Don't include it in your input schema to Dgraph - use your editing environment to set it up as an import.  The details will depend on your setup.

```graphql
# The Int64 scalar type represents a signed 64‐bit numeric non‐fractional value.
# Int64 can represent values in range [-(2^63),(2^63 - 1)].
scalar Int64

# The DateTime scalar type represents date and time as a string in RFC3339 format.
# For example: "1985-04-12T23:20:50.52Z" represents 20 minutes and 50.52 seconds after the 23rd hour of April 12th, 1985 in UTC.
scalar DateTime

input IntRange{
	min: Int
	max: Int
}

input FloatRange{
	min: Float
	max: Float
}

input Int64Range{
	min: Int64
	max: Int64
}

input DateTimeRange{
	min: DateTime
	max: DateTime
}

input StringRange{
	min: String
	max: String
}

enum DgraphIndex {
	int
	int64
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
	geo
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

type Point {
	longitude: Float!
	latitude: Float!
}

input PointRef {
	longitude: Float!
	latitude: Float!
}

input NearFilter {
	distance: Float!
	coordinate: PointRef!
}

input PointGeoFilter {
	near: NearFilter
	within: WithinFilter
}

type PointList {
	points: [Point!]!
}

input PointListRef {
	points: [PointRef!]!
}

type Polygon {
	coordinates: [PointList!]!
}

input PolygonRef {
	coordinates: [PointListRef!]!
}

type MultiPolygon {
	polygons: [Polygon!]!
}

input MultiPolygonRef {
	polygons: [PolygonRef!]!
}

input WithinFilter {
	polygon: PolygonRef!
}

input ContainsFilter {
	point: PointRef
	polygon: PolygonRef
}

input IntersectsFilter {
	polygon: PolygonRef
	multiPolygon: MultiPolygonRef
}

input PolygonGeoFilter {
	near: NearFilter
	within: WithinFilter
	contains: ContainsFilter
	intersects: IntersectsFilter
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
	delete:AuthRule) on OBJECT
directive @custom(http: CustomHTTP, dql: String) on FIELD_DEFINITION
directive @remote on OBJECT | INTERFACE | UNION | INPUT_OBJECT | ENUM
directive @cascade(fields: [String]) on FIELD
directive @lambda on FIELD_DEFINITION

input IntFilter {
	eq: Int
	le: Int
	lt: Int
	ge: Int
	gt: Int
	between: IntRange
}

input Int64Filter {
	eq: Int64
	le: Int64
	lt: Int64
	ge: Int64
	gt: Int64
	between: Int64Range
}

input FloatFilter {
	eq: Float
	le: Float
	lt: Float
	ge: Float
	gt: Float
	between: FloatRange
}

input DateTimeFilter {
	eq: DateTime
	le: DateTime
	lt: DateTime
	ge: DateTime
	gt: DateTime
	between: DateTimeRange
}

input StringTermFilter {
	allofterms: String
	anyofterms: String
}

input StringRegExpFilter {
	regexp: String
}

input StringFullTextFilter {
	alloftext: String
	anyoftext: String
}

input StringExactFilter {
	eq: String
	in: [String]
	le: String
	lt: String
	ge: String
	gt: String
	between: StringRange
}

input StringHashFilter {
	eq: String
	in: [String]
}
```
