# Nested Insert @auth Fix

This branch fixes a **false accept** security gap in Dgraph GraphQL mutations: nested inserts and `@hasInverse` edge linking could modify protected nodes without running the parent type's update authorization rules.

## Problem

When a mutation links a new child to an **existing parent** via `@hasInverse`, Dgraph mutates the parent's inverse predicate but only ran `@auth add` checks on **newly allocated UIDs**. Existing parents were never validated, so non-admin users could run mutations like:

```graphql
mutation {
  addFooItem(input: [{ parent: { id: "foo1" } }]) { numUids }
}
```

…even when `ProtectedFoo` requires admin for both add and update.

Deep nested adds could also create leaf nodes whose stricter `@auth add` rules were not enforced consistently.

## Fix

1. **`mutation_rewriter.go`**: track `AffectedNodes` when `asIDReference()` links through an inverse edge to an existing UID.
2. **`mutation.go`**: after `authorizeNewNodes()` (add rules on new UIDs), run `authorizeAffectedNodes()` using **update rules** on affected existing nodes. This runs for **add mutations only** to avoid breaking update mutation flows.

## Verify locally

### Unit tests

```bash
go test ./graphql/resolve -run 'TestAuthQueryRewriting|TestAddChildRecordsAffectedProtectedParent|TestNestedAddRecordsDeepNewNodes' -count=1
```

### E2E integration tests

Build the patched binary and local Docker image, then run auth e2e tests:

```bash
make dgraph
make local-image
docker tag dgraph/dgraph:local dgraph/dgraph:nested-auth-fix

# From graphql/e2e/auth (uses dgraph/dgraph:local via docker-compose.yml)
cd graphql/e2e/auth
docker compose up -d
go test -tags=integration -run TestNestedAdd -count=1 .
docker compose down
```

Or via the repo Makefile:

```bash
make test TAGS=integration PKG=graphql/e2e/auth TEST=TestNestedAdd
```

### Manual repro harness

1. Start Dgraph standalone on port 8080 (see Docker section below).
2. Run:

```bash
chmod +x .local/mint-jwt.sh .local/repro.sh
./.local/repro.sh
```

JWT helper:

```bash
./.local/mint-jwt.sh USER user@example.com false   # non-admin token
./.local/mint-jwt.sh ADMIN user@example.com true   # admin token
```

Secret lives in `.local/jwt-secret` (gitignored).

## Docker image

Build and tag the patched image:

```bash
make local-image
docker tag dgraph/dgraph:local dgraph/dgraph:nested-auth-fix
```

Use in docker-compose (override snippet):

```yaml
services:
  dgraph:
    image: dgraph/dgraph:nested-auth-fix
    ports:
      - "8080:8080"
      - "9080:9080"
    command: dgraph zero  # use standalone or zero+alpha layout for your setup
```

For raggen-style standalone on port 8080, point the `dgraph` service image at `dgraph/dgraph:nested-auth-fix` instead of `dgraph/dgraph:v25.2.0`.

## References

- [Parent update rules bypassed via child + hasInverse](https://discuss.dgraph.io/t/bug-auth-rules-of-parent-not-respected-when-child-with-hasinverse-is-added/12955)
- Branch: `fix/nested-auth-insertions` (based on `v25.3.5`)
