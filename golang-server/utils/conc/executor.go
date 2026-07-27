package conc

import (
	"context"
	"fmt"
)

// Executor handles concurrent execution of tasks with configurable behavior.
type Executor[T any] struct {
	config ExecutorConfig
	funcs  []ExecuteFunc[T]
}

// NewExecutor creates a new executor with the given configuration.
func NewExecutor[T any](config ExecutorConfig) *Executor[T] {
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 5 // Default max concurrent tasks
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 10 // Default buffer size
	}

	return &Executor[T]{
		config: config,
		funcs:  make([]ExecuteFunc[T], 0),
	}
}

// WithExecuteFunc adds an ExecuteFunc to the executor's function list.
func (e *Executor[T]) WithExecuteFunc(fn ExecuteFunc[T]) *Executor[T] {
	e.funcs = append(e.funcs, fn)
	return e
}

// WithExecuteFuncs adds multiple ExecuteFuncs to the executor's function list.
func (e *Executor[T]) WithExecuteFuncs(funcs ...ExecuteFunc[T]) *Executor[T] {
	e.funcs = append(e.funcs, funcs...)
	return e
}

// Run executes all added functions concurrently according to the configured behavior.
func (e *Executor[T]) Run(ctx context.Context) (<-chan TaskResult[T], error) {
	return e.execute(ctx, e.funcs)
}

// execute executes functions concurrently according to the configured behavior.
func (e *Executor[T]) execute(
	ctx context.Context,
	funcs []ExecuteFunc[T],
) (<-chan TaskResult[T], error) {
	if len(funcs) == 0 {
		// Return a closed channel for empty task list
		resultCh := make(chan TaskResult[T])
		close(resultCh)
		return resultCh, nil
	}

	// Create the result channel that will be returned to the user
	resultCh := make(chan TaskResult[T], e.config.BufferSize)

	// Use a semaphore to limit concurrent executions
	semaphore := make(chan struct{}, e.config.MaxConcurrent)

	// Choose execution strategy based on streaming mode
	switch e.config.StreamingMode {
	case StreamInOrder:
		e.executeInOrder(ctx, funcs, semaphore, resultCh)
	case StreamAsReady:
		e.executeAsReady(ctx, funcs, semaphore, resultCh)
	default:
		close(resultCh)
		return resultCh, fmt.Errorf("unsupported streaming mode: %d", e.config.StreamingMode)
	}

	return resultCh, nil
}

// executeInOrder executes functions and streams results in original order.
func (e *Executor[T]) executeInOrder(
	ctx context.Context,
	funcs []ExecuteFunc[T],
	semaphore chan struct{},
	resultCh chan TaskResult[T],
) {
	// Create individual channels for each function result
	funcChannels := make([]chan TaskResult[T], len(funcs))
	for i := range funcChannels {
		funcChannels[i] = make(chan TaskResult[T], 1)
	}

	// Start all functions concurrently
	for i, fn := range funcs {
		go e.executeFunc(ctx, fn, i, semaphore, funcChannels[i])
	}

	// Start goroutine to stream results in order
	go func() {
		defer close(resultCh)
		e.streamResultsInOrder(ctx, funcChannels, resultCh)
	}()
}

// executeAsReady executes functions and streams results as soon as they complete.
func (e *Executor[T]) executeAsReady(
	ctx context.Context,
	funcs []ExecuteFunc[T],
	semaphore chan struct{},
	resultCh chan TaskResult[T],
) {
	// Start goroutine to manage completion and channel closing
	go func() {
		defer close(resultCh)

		// Use a shared channel for all function results
		funcResultCh := make(chan TaskResult[T], len(funcs))

		// Start all functions concurrently
		for i, fn := range funcs {
			go e.executeFunc(ctx, fn, i, semaphore, funcResultCh)
		}

		// Stream results as they arrive
		e.streamResultsAsReady(ctx, len(funcs), funcResultCh, resultCh)
	}()
}

// executeFunc runs a single function with semaphore control.
func (e *Executor[T]) executeFunc(
	ctx context.Context,
	fn ExecuteFunc[T],
	position int,
	semaphore chan struct{},
	resultCh chan<- TaskResult[T],
) {
	// Acquire semaphore to limit concurrent executions
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		select {
		case resultCh <- TaskResult[T]{Position: position, Error: ctx.Err()}:
		case <-ctx.Done():
		}
		return
	}

	// Execute the function
	result, err := fn(ctx)

	// Send result
	taskResult := TaskResult[T]{
		Position: position,
		Result:   result,
		Error:    err,
	}

	select {
	case resultCh <- taskResult:
	case <-ctx.Done():
		// Context cancelled, but we still need to send the result to avoid deadlock
		select {
		case resultCh <- TaskResult[T]{Position: position, Error: ctx.Err()}:
		default:
		}
	}
}

// streamResultsInOrder waits for each result in order and streams immediately when ready.
func (e *Executor[T]) streamResultsInOrder(
	ctx context.Context,
	funcChannels []chan TaskResult[T],
	resultCh chan<- TaskResult[T],
) {
	// Process results in order (0, 1, 2, ...) but stream each as soon as it's ready
	for _, funcCh := range funcChannels {
		var result TaskResult[T]
		select {
		case result = <-funcCh:
			// Function is ready
		case <-ctx.Done():
			return
		}

		// Stream the result immediately
		select {
		case resultCh <- result:
		case <-ctx.Done():
			return
		}

		// Handle error based on configuration
		if result.Error != nil && e.config.ErrorHandling == StopOnError {
			return // Stop processing remaining functions
		}
		// Continue processing if ContinueOnError
	}
}

// streamResultsAsReady streams results as soon as they complete (potentially out of order).
func (e *Executor[T]) streamResultsAsReady(
	ctx context.Context,
	totalFuncs int,
	funcResultCh <-chan TaskResult[T],
	resultCh chan<- TaskResult[T],
) {
	completedFuncs := 0

	for completedFuncs < totalFuncs {
		select {
		case result := <-funcResultCh:
			completedFuncs++

			// Stream the result immediately
			select {
			case resultCh <- result:
			case <-ctx.Done():
				return
			}

			// Handle error based on configuration
			if result.Error != nil && e.config.ErrorHandling == StopOnError {
				return // Stop processing remaining functions
			}
			// Continue processing if ContinueOnError

		case <-ctx.Done():
			return
		}
	}
}
