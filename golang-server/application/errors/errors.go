package xerrors

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
)

type Error struct {
	error

	Code uint32
}

var (
	CodeNotFound           = uint32(connect.CodeNotFound)
	CodeBadRequest         = uint32(connect.CodeInvalidArgument)
	CodePermissionDenied   = uint32(connect.CodePermissionDenied)
	CodeInternal           = uint32(connect.CodeInternal)
	CodeConflict           = uint32(connect.CodeAlreadyExists)
	CodePreconditionFailed = uint32(connect.CodeFailedPrecondition)
)

// Sentinel errors for type checking.
var (
	ErrNotFound           = &Error{error: errors.New("not found"), Code: CodeNotFound}
	ErrBadRequest         = &Error{error: errors.New("bad request"), Code: CodeBadRequest}
	ErrPermissionDenied   = &Error{error: errors.New("permission denied"), Code: CodePermissionDenied}
	ErrInternal           = &Error{error: errors.New("internal error"), Code: CodeInternal}
	ErrConflict           = &Error{error: errors.New("conflict"), Code: CodeConflict}
	ErrPreconditionFailed = &Error{error: errors.New("precondition failed"), Code: CodePreconditionFailed}
)

func NotFoundError(ctx context.Context, err error) Error {
	return Error{
		error: fmt.Errorf("not found: %w", err),
		Code:  CodeNotFound,
	}
}

func BadRequestError(ctx context.Context, err error) Error {
	return Error{
		error: fmt.Errorf("bad request: %w", err),
		Code:  CodeBadRequest,
	}
}

func PermissionDeniedError(ctx context.Context, err error) Error {
	return Error{
		error: fmt.Errorf("permission denied: %w", err),
		Code:  CodePermissionDenied,
	}
}

func InternalError(ctx context.Context, err error) Error {
	zerolog.Ctx(ctx).Error().Err(err).Msg("internal error")
	return Error{
		// internal error is not exposed to client.
		// We might need to add application error code for specific errors.
		error: errors.New("internal error"),
		Code:  CodeInternal,
	}
}

func ConflictError(ctx context.Context, err error) Error {
	return Error{
		error: fmt.Errorf("conflict: %w", err),
		Code:  CodeConflict,
	}
}

func PreconditionFailedError(ctx context.Context, err error) Error {
	return Error{
		error: fmt.Errorf("precondition failed: %w", err),
		Code:  CodePreconditionFailed,
	}
}

func IsNotFoundError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodeNotFound
	}
	return false
}

func IsBadRequestError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodeBadRequest
	}
	return false
}

func IsPermissionDeniedError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodePermissionDenied
	}
	return false
}

func IsInternalError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodeInternal
	}
	return false
}

func IsConflictError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodeConflict
	}
	return false
}

func IsPreconditionFailedError(err error) bool {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code == CodePreconditionFailed
	}
	return false
}

func GetErrorCode(err error) uint32 {
	var xerr Error
	if errors.As(err, &xerr) {
		return xerr.Code
	}
	return CodeInternal
}

// Unwrap returns the underlying error.
func (e Error) Unwrap() error {
	return e.error
}

func ToConnectError(ctx context.Context, err error) *connect.Error {
	var xerr Error
	if errors.As(err, &xerr) {
		return connect.NewError(connect.Code(xerr.Code), xerr.error)
	}

	zerolog.Ctx(ctx).Error().Err(err).Msg("internal error")
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// NewErrorInterceptor converts returned errors to Connect errors.
func NewErrorInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}

			// If the error is already a Connect error
			var connectErr *connect.Error
			if errors.As(err, &connectErr) {
				return resp, err
			}

			return resp, ToConnectError(ctx, err)
		}
	}
}
