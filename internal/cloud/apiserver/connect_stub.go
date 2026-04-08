// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
//
// This file contains a hand-rolled subset of the connectrpc.com/connect API.
// The real package isn't a direct dependency yet (KAI-238 authored the proto
// schemas but `buf generate` has not been run), so the middleware stack and
// service handlers in this package use the local shims below. When KAI-310
// wires proto generation, this file is deleted and callers switch to the
// real imports:
//
//	import "connectrpc.com/connect"
//
// The shapes here intentionally mirror the upstream API so the migration is
// a near-mechanical s/apiserver.Code/connect.Code/.
package apiserver

import (
	"fmt"
)

// Code is a lightweight mirror of connect.Code. Only the values used by the
// placeholder service handlers are listed; add more on demand.
type Code int

const (
	CodeUnknown         Code = 2
	CodeInvalidArgument Code = 3
	CodeNotFound        Code = 5
	CodeUnauthenticated Code = 16
	CodePermissionDenied Code = 7
	CodeUnimplemented   Code = 12
	CodeInternal        Code = 13
	CodeResourceExhausted Code = 8
)

// String returns the canonical textual name for the code.
func (c Code) String() string {
	switch c {
	case CodeUnknown:
		return "unknown"
	case CodeInvalidArgument:
		return "invalid_argument"
	case CodeNotFound:
		return "not_found"
	case CodeUnauthenticated:
		return "unauthenticated"
	case CodePermissionDenied:
		return "permission_denied"
	case CodeUnimplemented:
		return "unimplemented"
	case CodeInternal:
		return "internal"
	case CodeResourceExhausted:
		return "resource_exhausted"
	default:
		return fmt.Sprintf("code(%d)", int(c))
	}
}

// HTTPStatus maps a connect Code to an HTTP status for the JSON error
// envelope emitted by this server. Matches connect-go's default mapping.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeInvalidArgument:
		return 400
	case CodeUnauthenticated:
		return 401
	case CodePermissionDenied:
		return 403
	case CodeNotFound:
		return 404
	case CodeResourceExhausted:
		return 429
	case CodeUnimplemented:
		return 501
	case CodeInternal, CodeUnknown:
		return 500
	default:
		return 500
	}
}

// ConnectError mirrors *connect.Error. It implements error and carries a
// stable Code used by the JSON envelope writer.
type ConnectError struct {
	code Code
	err  error
}

// NewError is the stub counterpart of connect.NewError.
func NewError(code Code, err error) *ConnectError {
	return &ConnectError{code: code, err: err}
}

// Code returns the connect code.
func (e *ConnectError) Code() Code { return e.code }

// Error implements the standard error interface.
func (e *ConnectError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.code.String(), e.err.Error())
}

// Unwrap returns the inner error for errors.Is/As.
func (e *ConnectError) Unwrap() error { return e.err }
