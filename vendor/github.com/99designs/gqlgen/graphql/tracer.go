package graphql

import (
	"context"
)

var _ Tracer = (*NopTracer)(nil)

type Tracer interface {
	StartOperationParsing(ctx context.Context) context.Context
	EndOperationParsing(ctx context.Context)
	StartOperationValidation(ctx context.Context) context.Context
	EndOperationValidation(ctx context.Context)
	StartOperationExecution(ctx context.Context) context.Context
	StartFieldExecution(ctx context.Context, field CollectedField) context.Context
	StartFieldResolverExecution(ctx context.Context, rc *ResolverContext) context.Context
	StartFieldChildExecution(ctx context.Context) context.Context
	EndFieldExecution(ctx context.Context)
	EndOperationExecution(ctx context.Context)
}

type NopTracer struct{}

func (NopTracer) StartOperationParsing(ctx context.Context) context.Context {
	return ctx
}

func (NopTracer) EndOperationParsing(ctx context.Context) {
}

func (NopTracer) StartOperationValidation(ctx context.Context) context.Context {
	return ctx
}

func (NopTracer) EndOperationValidation(ctx context.Context) {
}

func (NopTracer) StartOperationExecution(ctx context.Context) context.Context {
	return ctx
}

func (NopTracer) StartFieldExecution(ctx context.Context, field CollectedField) context.Context {
	return ctx
}

func (NopTracer) StartFieldResolverExecution(ctx context.Context, rc *ResolverContext) context.Context {
	return ctx
}

func (NopTracer) StartFieldChildExecution(ctx context.Context) context.Context {
	return ctx
}

func (NopTracer) EndFieldExecution(ctx context.Context) {
}

func (NopTracer) EndOperationExecution(ctx context.Context) {
}
