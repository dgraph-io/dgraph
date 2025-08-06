# DQL (Dgraph Query Language) Syntax - Multi-Document Context

## Overview

DQL is Dgraph's query language for graph databases, supporting queries, mutations, and schema
operations. This document provides a comprehensive syntax reference based on the lexer and parser
implementation.

## Token Types (from `dql/state.go`)

### Basic Tokens

- `itemText` - Plain text content
- `itemName` - Identifiers, predicates, variable names
- `itemOpType` - Operation types (query, mutation, schema, fragment)

### Structural Tokens

- `itemLeftCurl` / `itemRightCurl` - `{` / `}` (blocks)
- `itemLeftRound` / `itemRightRound` - `(` / `)` (function arguments)
- `itemLeftSquare` / `itemRightSquare` - `[` / `]` (arrays)
- `itemComma` - `,` (separators)
- `itemColon` - `:` (key-value pairs)
- `itemPeriod` - `.` (object navigation)

### Special Tokens

- `itemAt` - `@` (directives)
- `itemDollar` - `$` (variables)
- `itemEqual` - `=` (assignments)
- `itemStar` - `*` (wildcards)
- `itemRegex` - `/pattern/flags` (regular expressions)
- `itemMathOp` - `+`, `-`, `*`, `/`, `%` (mathematical operations)

### Mutation Tokens

- `itemMutationOp` - Mutation operations (`set`, `delete`)
- `itemMutationOpContent` - Content within mutation blocks
- `itemUpsertBlock` - Upsert operation blocks
- `itemUpsertBlockOp` - Upsert operations (`query`, `mutation`)

## Core Syntax Elements

### 1. Query Structure

```dql
{
  predicate(func: function_name(args)) @directive {
    sub_predicate
    alias: another_predicate
  }
}
```

### 2. Functions (from `dql/parser.go`)

- **Root Functions**: `uid()`, `eq()`, `le()`, `ge()`, `gt()`, `lt()`, `between()`
- **Aggregation**: `min()`, `max()`, `sum()`, `avg()`, `count()`
- **Special**: `type()`, `val()`, `len()`, `uid_in()`, `similar_to()`
- **Geo Functions**: `near()`, `contains()`, `within()`, `intersects()`

### 3. Directives (prefixed with `@`)

- `@filter()` - Apply filters to results
- `@facets()` - Include edge facets
- `@groupby()` - Group results
- `@normalize` - Normalize output structure
- `@cascade` - Remove nodes without all requested fields
- `@recurse()` - Recursive traversal
- `@if()` - Conditional mutations

### 4. Variables

- **Query Variables**: `var_name as predicate`
- **Value Variables**: `$variable_name`
- **UID Variables**: References to previously defined variables

### 5. Filters

```dql
@filter(func_name(predicate, value) AND/OR/NOT other_conditions)
```

### 6. Facets

```dql
predicate @facets(facet_key)
predicate @facets(eq(facet_key, value))
```

### 7. Sorting and Pagination

```dql
predicate(orderasc: field, orderdesc: field, first: N, offset: N)
```

## Mutation Syntax

### 1. Set Mutations

```dql
{
  set {
    <uid> <predicate> "value" .
    <uid> <predicate> <other_uid> .
  }
}
```

### 2. Delete Mutations

```dql
{
  delete {
    <uid> <predicate> * .
    <uid> * * .
  }
}
```

### 3. Upsert Operations

```dql
upsert {
  query {
    # Query block
  }
  mutation {
    # Mutation block with @if conditions
  }
}
```

## Schema Operations

### 1. Schema Definition

```dql
schema {
  predicate: type @directive .
  type TypeName {
    field1
    field2: type
  }
}
```

### 2. Schema Directives

- `@index(tokenizer)` - Create index
- `@upsert` - Enable upsert operations
- `@unique` - Enforce uniqueness
- `@count` - Enable count queries
- `@reverse` - Create reverse edges

## Data Types

- `string` - Text data
- `int` - Integer numbers
- `float` - Floating-point numbers
- `bool` - Boolean values
- `datetime` - Date and time
- `geo` - Geographical data
- `uid` - Node references
- `[type]` - Arrays of specified type

## Mathematical Expressions

```dql
math(expression) {
  # Mathematical operations using +, -, *, /, %
  # Can reference variables and predicates
}
```

## Language Tags

```dql
predicate@en  # English
predicate@.   # All languages
predicate@*   # Any language
```

## Comments

```dql
# Single line comments start with hash
```

## Character Classes (from lexer rules)

### Name Characters

- **Start**: `a-z`, `A-Z`, `_`, `~`
- **Continuation**: Letters, numbers, `.`, `_`, `~`

### String Literals

- Enclosed in double quotes: `"string"`
- Escape sequences: `\u`, `\v`, `\t`, `\b`, `\n`, `\r`, `\f`, `\"`, `\'`, `\\`

### Regular Expressions

- Pattern: `/regex_pattern/flags`
- Flags: `a-z`, `A-Z`

## Parser Structure (from `dql/parser.go`)

### GraphQuery Components

- `UID` - Specific node UIDs
- `Attr` - Predicate name
- `Langs` - Language specifications
- `Alias` - Field aliases
- `Func` - Root function
- `Args` - Function arguments
- `Order` - Sorting specifications
- `Children` - Nested queries
- `Filter` - Filter conditions
- `Facets` - Facet specifications

### Variable Context Types

- `AnyVar` (0) - Any variable type
- `UidVar` (1) - UID variables
- `ValueVar` (2) - Value variables
- `ListVar` (3) - List variables

## Error Handling

The parser provides detailed error messages with line and column information for syntax errors,
invalid operations, and malformed queries.

## Best Practices

1. Use meaningful variable names
2. Leverage indexes for performance
3. Use facets for edge metadata
4. Apply filters early in query execution
5. Use upserts for conditional mutations
6. Validate schema before complex operations

---

_This MDC is based on the Dgraph v25 implementation as found in the lexer (`lex/lexer.go`), parser
(`dql/parser.go`), and state definitions (`dql/state.go`)._
