package conc

import "context"

// ExecuteFunc is a function that can be executed concurrently.
type ExecuteFunc[T any] func(ctx context.Context) (T, error)

// TaskResult holds the result of a task execution with its position.
type TaskResult[T any] struct {
	Position int
	Result   T
	Error    error
}

// ErrorHandling defines how to handle task errors.
type ErrorHandling int

const (
	// ContinueOnError continues executing remaining tasks when one fails.
	ContinueOnError ErrorHandling = iota
	// StopOnError stops execution and returns immediately when any task fails.
	StopOnError
)

// StreamingMode defines how results are streamed back.
type StreamingMode int

const (
	// StreamInOrder streams results in the original task order (wait for position 0, then 1, then 2...)
	StreamInOrder StreamingMode = iota
	// StreamAsReady streams results immediately as they complete (may be out of order).
	StreamAsReady
)

// ExecutorConfig holds configuration for parallel execution.
type ExecutorConfig struct {
	// MaxConcurrent limits the number of concurrent tasks
	MaxConcurrent int
	// BufferSize sets the buffer size for internal channels
	BufferSize int
	// ErrorHandling defines behavior when tasks fail
	ErrorHandling ErrorHandling
	// StreamingMode defines how results are delivered
	StreamingMode StreamingMode
}
