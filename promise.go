package gv8

// #include "gv8.h"
import "C"

import (
	"context"
	"errors"
	"time"
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
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
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
	release := r.ctx.iso.mustEnter()
	defer release()
	return &Promise{Value: newValue(r.ctx, C.GV8PromiseResolverGetPromise(r.ptr))}
}

func (r *PromiseResolver) Resolve(value *Value) error {
	if r == nil || !r.valid() {
		return invalidValueError()
	}
	if value == nil || !value.valid() {
		return invalidValueError()
	}
	release, err := r.ctx.iso.enter()
	if err != nil {
		return err
	}
	defer release()
	return newJSError(C.GV8PromiseResolverResolve(r.ptr, value.ptr))
}

func (r *PromiseResolver) Reject(value *Value) error {
	if r == nil || !r.valid() {
		return invalidValueError()
	}
	if value == nil || !value.valid() {
		return invalidValueError()
	}
	release, err := r.ctx.iso.enter()
	if err != nil {
		return err
	}
	defer release()
	return newJSError(C.GV8PromiseResolverReject(r.ptr, value.ptr))
}

func (p *Promise) State() PromiseState {
	if p == nil || !p.valid() {
		return PromisePending
	}
	release := p.ctx.iso.mustEnter()
	defer release()
	return PromiseState(C.GV8PromiseState(p.ptr))
}

func (p *Promise) Result() *Value {
	if p == nil || !p.valid() {
		return nil
	}
	release := p.ctx.iso.mustEnter()
	defer release()
	return newValue(p.ctx, C.GV8PromiseResult(p.ptr))
}

func (p *Promise) Await(ctx context.Context, pump func(context.Context) error) (*Value, error) {
	if p == nil {
		return nil, errors.New("gv8: nil promise")
	}
	if !p.valid() {
		return nil, invalidValueError()
	}

	for {
		if !p.valid() {
			return nil, invalidValueError()
		}
		if p.ctx != nil && p.ctx.iso != nil {
			p.ctx.iso.PerformMicrotaskCheckpoint()
		}
		if p.State() != PromisePending {
			break
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		if pump != nil {
			if err := pump(ctx); err != nil {
				return nil, err
			}
			continue
		}
		timer := time.NewTimer(time.Millisecond)
		if ctx == nil {
			<-timer.C
			continue
		}
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	switch p.State() {
	case PromiseFulfilled:
		return p.Result(), nil
	case PromiseRejected:
		result := p.Result()
		if result == nil {
			return nil, &JSError{Message: "promise rejected"}
		}
		message, err := result.StringValue()
		if err != nil || message == "" {
			return nil, &JSError{Message: "promise rejected"}
		}
		return nil, &JSError{Message: "promise rejected: " + message}
	default:
		return nil, errors.New("promise did not settle")
	}
}
