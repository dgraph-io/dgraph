## Brief overview

These guidelines are specific to testing preferences for Go projects, particularly for the dgraph
repository, focusing on direct Go testing rather than Docker-based make targets.

## Testing approach

- Use `go test -v` with specific package paths instead of the make system for running tests
- Run tests on individual packages like `go test -v ./algo/...`, `go test -v ./chunker/...`,
  `go test -v ./codec/...`
- Avoid `make test` which requires Docker and can have dependency issues
- Direct Go testing works reliably without external dependencies like Docker containers

## Development workflow

- When asked to run tests, start with unit tests using `go test -v` on specific packages
- Only resort to Docker-based integration tests if specifically requested
- Test individual components first to quickly identify issues without Docker overhead

## Environment considerations

- Assume Docker may not be available or running on the development machine
- Prefer lightweight unit tests that don't require external services
- Use Go's built-in testing framework directly rather than complex build systems
