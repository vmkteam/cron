package cron

import (
	"context"
)

type ctxKey struct{}

type ContextValue[K ~struct{}, T any] struct{}

func NewContextValue[K ~struct{}, T any]() *ContextValue[K, T] {
	return &ContextValue[K, T]{}
}

func (p *ContextValue[K, T]) WithValue(ctx context.Context, v T) context.Context {
	return context.WithValue(ctx, K{}, v)
}

func (p *ContextValue[K, T]) FromContext(ctx context.Context) T {
	v, _ := ctx.Value(K{}).(T)
	return v
}
