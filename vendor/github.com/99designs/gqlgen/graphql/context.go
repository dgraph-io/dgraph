package graphql

import (
	"context"
	"fmt"
	"sync"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

type Resolver func(ctx context.Context) (res interface{}, err error)
type FieldMiddleware func(ctx context.Context, next Resolver) (res interface{}, err error)
type RequestMiddleware func(ctx context.Context, next func(ctx context.Context) []byte) []byte
type ComplexityLimitFunc func(ctx context.Context) int

type RequestContext struct {
	RawQuery  string
	Variables map[string]interface{}
	Doc       *ast.QueryDocument

	ComplexityLimit      int
	OperationComplexity  int
	DisableIntrospection bool

	// ErrorPresenter will be used to generate the error
	// message from errors given to Error().
	ErrorPresenter      ErrorPresenterFunc
	Recover             RecoverFunc
	ResolverMiddleware  FieldMiddleware
	DirectiveMiddleware FieldMiddleware
	RequestMiddleware   RequestMiddleware
	Tracer              Tracer

	errorsMu     sync.Mutex
	Errors       gqlerror.List
	extensionsMu sync.Mutex
	Extensions   map[string]interface{}
}

func DefaultResolverMiddleware(ctx context.Context, next Resolver) (res interface{}, err error) {
	return next(ctx)
}

func DefaultDirectiveMiddleware(ctx context.Context, next Resolver) (res interface{}, err error) {
	return next(ctx)
}

func DefaultRequestMiddleware(ctx context.Context, next func(ctx context.Context) []byte) []byte {
	return next(ctx)
}

func NewRequestContext(doc *ast.QueryDocument, query string, variables map[string]interface{}) *RequestContext {
	return &RequestContext{
		Doc:                 doc,
		RawQuery:            query,
		Variables:           variables,
		ResolverMiddleware:  DefaultResolverMiddleware,
		DirectiveMiddleware: DefaultDirectiveMiddleware,
		RequestMiddleware:   DefaultRequestMiddleware,
		Recover:             DefaultRecover,
		ErrorPresenter:      DefaultErrorPresenter,
		Tracer:              &NopTracer{},
	}
}

type key string

const (
	request  key = "request_context"
	resolver key = "resolver_context"
)

func GetRequestContext(ctx context.Context) *RequestContext {
	if val, ok := ctx.Value(request).(*RequestContext); ok {
		return val
	}
	return nil
}

func WithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, request, rc)
}

type ResolverContext struct {
	Parent *ResolverContext
	// The name of the type this field belongs to
	Object string
	// These are the args after processing, they can be mutated in middleware to change what the resolver will get.
	Args map[string]interface{}
	// The raw field
	Field CollectedField
	// The index of array in path.
	Index *int
	// The result object of resolver
	Result interface{}
	// IsMethod indicates if the resolver is a method
	IsMethod bool
}

func (r *ResolverContext) Path() []interface{} {
	var path []interface{}
	for it := r; it != nil; it = it.Parent {
		if it.Index != nil {
			path = append(path, *it.Index)
		} else if it.Field.Field != nil {
			path = append(path, it.Field.Alias)
		}
	}

	// because we are walking up the chain, all the elements are backwards, do an inplace flip.
	for i := len(path)/2 - 1; i >= 0; i-- {
		opp := len(path) - 1 - i
		path[i], path[opp] = path[opp], path[i]
	}

	return path
}

func GetResolverContext(ctx context.Context) *ResolverContext {
	if val, ok := ctx.Value(resolver).(*ResolverContext); ok {
		return val
	}
	return nil
}

func WithResolverContext(ctx context.Context, rc *ResolverContext) context.Context {
	rc.Parent = GetResolverContext(ctx)
	return context.WithValue(ctx, resolver, rc)
}

// This is just a convenient wrapper method for CollectFields
func CollectFieldsCtx(ctx context.Context, satisfies []string) []CollectedField {
	resctx := GetResolverContext(ctx)
	return CollectFields(GetRequestContext(ctx), resctx.Field.Selections, satisfies)
}

// CollectAllFields returns a slice of all GraphQL field names that were selected for the current resolver context.
// The slice will contain the unique set of all field names requested regardless of fragment type conditions.
func CollectAllFields(ctx context.Context) []string {
	resctx := GetResolverContext(ctx)
	collected := CollectFields(GetRequestContext(ctx), resctx.Field.Selections, nil)
	uniq := make([]string, 0, len(collected))
Next:
	for _, f := range collected {
		for _, name := range uniq {
			if name == f.Name {
				continue Next
			}
		}
		uniq = append(uniq, f.Name)
	}
	return uniq
}

// Errorf sends an error string to the client, passing it through the formatter.
func (c *RequestContext) Errorf(ctx context.Context, format string, args ...interface{}) {
	c.errorsMu.Lock()
	defer c.errorsMu.Unlock()

	c.Errors = append(c.Errors, c.ErrorPresenter(ctx, fmt.Errorf(format, args...)))
}

// Error sends an error to the client, passing it through the formatter.
func (c *RequestContext) Error(ctx context.Context, err error) {
	c.errorsMu.Lock()
	defer c.errorsMu.Unlock()

	c.Errors = append(c.Errors, c.ErrorPresenter(ctx, err))
}

// HasError returns true if the current field has already errored
func (c *RequestContext) HasError(rctx *ResolverContext) bool {
	c.errorsMu.Lock()
	defer c.errorsMu.Unlock()
	path := rctx.Path()

	for _, err := range c.Errors {
		if equalPath(err.Path, path) {
			return true
		}
	}
	return false
}

// GetErrors returns a list of errors that occurred in the current field
func (c *RequestContext) GetErrors(rctx *ResolverContext) gqlerror.List {
	c.errorsMu.Lock()
	defer c.errorsMu.Unlock()
	path := rctx.Path()

	var errs gqlerror.List
	for _, err := range c.Errors {
		if equalPath(err.Path, path) {
			errs = append(errs, err)
		}
	}
	return errs
}

func equalPath(a []interface{}, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// AddError is a convenience method for adding an error to the current response
func AddError(ctx context.Context, err error) {
	GetRequestContext(ctx).Error(ctx, err)
}

// AddErrorf is a convenience method for adding an error to the current response
func AddErrorf(ctx context.Context, format string, args ...interface{}) {
	GetRequestContext(ctx).Errorf(ctx, format, args...)
}

// RegisterExtension registers an extension, returns error if extension has already been registered
func (c *RequestContext) RegisterExtension(key string, value interface{}) error {
	c.extensionsMu.Lock()
	defer c.extensionsMu.Unlock()

	if c.Extensions == nil {
		c.Extensions = make(map[string]interface{})
	}

	if _, ok := c.Extensions[key]; ok {
		return fmt.Errorf("extension already registered for key %s", key)
	}

	c.Extensions[key] = value
	return nil
}

// ChainFieldMiddleware add chain by FieldMiddleware
func ChainFieldMiddleware(handleFunc ...FieldMiddleware) FieldMiddleware {
	n := len(handleFunc)

	if n > 1 {
		lastI := n - 1
		return func(ctx context.Context, next Resolver) (interface{}, error) {
			var (
				chainHandler Resolver
				curI         int
			)
			chainHandler = func(currentCtx context.Context) (interface{}, error) {
				if curI == lastI {
					return next(currentCtx)
				}
				curI++
				res, err := handleFunc[curI](currentCtx, chainHandler)
				curI--
				return res, err

			}
			return handleFunc[0](ctx, chainHandler)
		}
	}

	if n == 1 {
		return handleFunc[0]
	}

	return func(ctx context.Context, next Resolver) (interface{}, error) {
		return next(ctx)
	}
}
