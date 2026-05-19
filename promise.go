package gv8

// #include "gv8.h"
import "C"

import (
	"context"
	"errors"
	"fmt"
)

type PromiseState int

const (
	PromisePending PromiseState = iota
	PromiseFulfilled
	PromiseRejected
)

type Promise struct {
	*Value
}

type PromiseResolver struct {
	*Value
}

func NewPromiseResolver(ctx *Context) (*PromiseResolver, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	rtn := C.GV8ContextNewPromiseResolver(ctx.ptr)
	value, err := valueResult(ctx, rtn)
	if err != nil {
		return nil, err
	}
	return &PromiseResolver{Value: value}, nil
}

func (r *PromiseResolver) Promise() *Promise {
	if r == nil || !r.valid() {
		return nil
	}
	return &Promise{Value: newValue(r.ctx, C.GV8PromiseResolverGetPromise(r.ptr))}
}

func (r *PromiseResolver) Resolve(value *Value) error {
	if r == nil || !r.valid() {
		return &JSError{Message: "gv8: value is no longer valid"}
	}
	if value == nil || !value.valid() {
		return &JSError{Message: "gv8: value is no longer valid"}
	}
	return newJSError(C.GV8PromiseResolverResolve(r.ptr, value.ptr))
}

func (r *PromiseResolver) Reject(value *Value) error {
	if r == nil || !r.valid() {
		return &JSError{Message: "gv8: value is no longer valid"}
	}
	if value == nil || !value.valid() {
		return &JSError{Message: "gv8: value is no longer valid"}
	}
	return newJSError(C.GV8PromiseResolverReject(r.ptr, value.ptr))
}

func (p *Promise) State() PromiseState {
	if p == nil || !p.valid() {
		return PromisePending
	}
	return PromiseState(C.GV8PromiseState(p.ptr))
}

func (p *Promise) Result() *Value {
	if p == nil || !p.valid() {
		return nil
	}
	return newValue(p.ctx, C.GV8PromiseResult(p.ptr))
}

func (p *Promise) Await(ctx context.Context, pump func(context.Context) error) (*Value, error) {
	if p == nil {
		return nil, errors.New("gv8: nil promise")
	}

	for {
		if p.ctx != nil && p.ctx.iso != nil {
			p.ctx.iso.PerformMicrotaskCheckpoint()
		}

		if p.State() != PromisePending {
			break
		}

		if pump == nil {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
			}
			continue
		}

		if err := pump(ctx); err != nil {
			return nil, err
		}
	}

	switch p.State() {
	case PromiseFulfilled:
		return p.Result(), nil
	case PromiseRejected:
		result := p.Result()
		if result == nil {
			return nil, errors.New("promise rejected")
		}
		return nil, fmt.Errorf("promise rejected: %s", result.String())
	default:
		return nil, errors.New("promise did not settle")
	}
}
