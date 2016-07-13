# jemalloc

This is a go-gettable version of the jemalloc allocator for use in Go code that
needs to link against jemalloc but wants to integrate with `go get` and
`go build`.

To use in your project you need to import the package and set appropriate cgo flag directives:

```
import _ "github.com/cockroachdb/c-jemalloc"

// #cgo CPPFLAGS: -I <relative-path>/c-jemalloc/internal/include
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all
import "C"
```
