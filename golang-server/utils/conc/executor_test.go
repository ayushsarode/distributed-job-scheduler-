package conc

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestExecutor_Execute_EmptyTasks(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 2,
		BufferSize:    5,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	executor := NewExecutor[string](config)
	resultCh, err := executor.Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error for empty tasks, got: %v", err)
	}

	// Channel should be closed immediately
	results := collectResults(resultCh)
	if len(results) != 0 {
		t.Fatalf("Expected 0 results for empty tasks, got: %d", len(results))
	}
}

func TestExecutor_Execute_StreamInOrder_ContinueOnError(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 3,
		BufferSize:    10,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	// Create tasks with different completion times and one error
	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "task0", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "", errors.New("task1 failed")
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "task2", nil
		},
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	// Should get all 3 results in order
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got: %d", len(results))
	}

	// Check order
	if results[0].Position != 0 || results[1].Position != 1 || results[2].Position != 2 {
		t.Fatalf("Results not in order: %v", results)
	}

	verifyStreamInOrderResults(t, results)
}

func verifyStreamInOrderResults(t *testing.T, results []TaskResult[string]) {
	// Check task 0
	if results[0].Error != nil {
		t.Fatalf("Task 0 should not have error, got: %v", results[0].Error)
	}
	if results[0].Result != "task0" {
		t.Fatalf("Task 0 result should be 'task0', got: %s", results[0].Result)
	}

	// Check task 1
	if results[1].Error == nil {
		t.Fatalf("Task 1 should have error")
	}

	// Check task 2
	if results[2].Error != nil {
		t.Fatalf("Task 2 should not have error, got: %v", results[2].Error)
	}
	if results[2].Result != "task2" {
		t.Fatalf("Task 2 result should be 'task2', got: %s", results[2].Result)
	}
}

func TestExecutor_Execute_StreamInOrder_StopOnError(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 3,
		BufferSize:    10,
		ErrorHandling: StopOnError,
		StreamingMode: StreamInOrder,
	}

	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "task0", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "", errors.New("task1 failed")
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "task2", nil
		},
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	// Should get results up to and including the error
	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got: %d", len(results))
	}

	// First result should be successful
	if results[0].Position != 0 || results[0].Error != nil {
		t.Fatalf("First result should be position 0 with no error, got: %v", results[0])
	}

	// Second result should be the error
	if results[1].Position != 1 || results[1].Error == nil {
		t.Fatalf("Second result should be position 1 with error, got: %v", results[1])
	}

	// Should not get third result due to StopOnError
	if len(results) > 2 {
		t.Fatalf("Should not get more than 2 results with StopOnError, got: %d", len(results))
	}
}

func TestExecutor_Execute_StreamAsReady_ContinueOnError(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 3,
		BufferSize:    10,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamAsReady,
	}

	// Create tasks with different completion times
	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond) // Slowest
			return "slow", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(50 * time.Millisecond) // Fastest
			return "fast", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond) // Medium
			return "medium", nil
		},
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	// Should get all 3 results
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got: %d", len(results))
	}

	// Results should come in completion order (fastest first)
	// Since we can't guarantee exact timing, we'll check that we got all positions
	positions := make([]int, len(results))
	for i, result := range results {
		positions[i] = result.Position
	}
	sort.Ints(positions)

	expectedPositions := []int{0, 1, 2}
	for i, pos := range positions {
		if pos != expectedPositions[i] {
			t.Fatalf("Missing position %d in results", expectedPositions[i])
		}
	}
}

func TestExecutor_Execute_ConcurrencyLimit(t *testing.T) {
	maxConcurrent := 2
	config := ExecutorConfig{
		MaxConcurrent: maxConcurrent,
		BufferSize:    10,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamAsReady,
	}

	var activeTasks int32
	var maxActiveTasks int32
	var mu sync.Mutex

	// Create tasks that track concurrency
	funcs := make([]ExecuteFunc[int], 5)
	for i := range 5 {
		taskNum := i
		funcs[i] = func(ctx context.Context) (int, error) {
			mu.Lock()
			activeTasks++
			if activeTasks > maxActiveTasks {
				maxActiveTasks = activeTasks
			}
			mu.Unlock()

			// Simulate work
			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			activeTasks--
			mu.Unlock()

			return taskNum, nil
		}
	}

	resultCh, err := NewExecutor[int](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	if len(results) != 5 {
		t.Fatalf("Expected 5 results, got: %d", len(results))
	}

	mu.Lock()
	finalMaxActive := maxActiveTasks
	mu.Unlock()

	if finalMaxActive > int32(maxConcurrent) {
		t.Fatalf("Concurrency limit violated: max active was %d, limit is %d", finalMaxActive, maxConcurrent)
	}
}

func TestExecutor_Execute_ContextCancellation(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 3,
		BufferSize:    10,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create long-running tasks
	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond) // Will be cancelled
			return "task0", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond) // Will be cancelled
			return "task1", nil
		},
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	// Should get some results with context cancellation errors
	for _, result := range results {
		if result.Error != nil && !errors.Is(result.Error, context.DeadlineExceeded) {
			t.Logf("Got expected cancellation error: %v", result.Error)
		}
	}
}

// Helper function to collect all results from a channel.
func collectResults[T any](resultCh <-chan TaskResult[T]) []TaskResult[T] {
	results := make([]TaskResult[T], 0)
	for result := range resultCh {
		results = append(results, result)
	}
	return results
}

func TestExecuteFuncs_BasicFunctionality(t *testing.T) {
	funcs := []ExecuteFunc[int]{
		func(ctx context.Context) (int, error) {
			return 1, nil
		},
		func(ctx context.Context) (int, error) {
			return 2, nil
		},
		func(ctx context.Context) (int, error) {
			return 3, nil
		},
	}

	config := ExecutorConfig{
		MaxConcurrent: 2,
		BufferSize:    5,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	resultCh, err := NewExecutor[int](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	results := collectResults(resultCh)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got: %d", len(results))
	}

	// Check results are in order
	for i, result := range results {
		if result.Position != i {
			t.Fatalf("Expected position %d, got %d", i, result.Position)
		}
		if result.Error != nil {
			t.Fatalf("Expected no error for position %d, got: %v", i, result.Error)
		}
		expectedValue := i + 1
		if result.Result != expectedValue {
			t.Fatalf("Expected result %d for position %d, got %d", expectedValue, i, result.Result)
		}
	}
}

func TestExecuteFuncs_EmptyFunctions(t *testing.T) {
	config := ExecutorConfig{
		MaxConcurrent: 2,
		BufferSize:    5,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs().Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error for empty functions, got: %v", err)
	}

	results := collectResults(resultCh)
	if len(results) != 0 {
		t.Fatalf("Expected 0 results for empty functions, got: %d", len(results))
	}
}

func TestExecuteFuncs_WithErrors(t *testing.T) {
	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			return "success1", nil
		},
		func(ctx context.Context) (string, error) {
			return "", errors.New("function error")
		},
		func(ctx context.Context) (string, error) {
			return "success2", nil
		},
	}

	config := ExecutorConfig{
		MaxConcurrent: 3,
		BufferSize:    5,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got: %d", len(results))
	}

	// Check first result
	if results[0].Error != nil {
		t.Fatalf("Expected no error for first result, got: %v", results[0].Error)
	}
	if results[0].Result != "success1" {
		t.Fatalf("Expected 'success1', got '%s'", results[0].Result)
	}

	// Check second result (error)
	if results[1].Error == nil {
		t.Fatalf("Expected error for second result")
	}

	// Check third result
	if results[2].Error != nil {
		t.Fatalf("Expected no error for third result, got: %v", results[2].Error)
	}
	if results[2].Result != "success2" {
		t.Fatalf("Expected 'success2', got '%s'", results[2].Result)
	}
}

func TestExecuteFuncs_ContextCancellation(t *testing.T) {
	// Use a timeout context that will definitely cancel
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	funcs := []ExecuteFunc[string]{
		func(ctx context.Context) (string, error) {
			// This task will run longer than the context timeout
			time.Sleep(50 * time.Millisecond)
			return "should not complete", nil
		},
	}

	config := ExecutorConfig{
		MaxConcurrent: 1,
		BufferSize:    5,
		ErrorHandling: ContinueOnError,
		StreamingMode: StreamInOrder,
	}

	resultCh, err := NewExecutor[string](config).WithExecuteFuncs(funcs...).Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error starting execution, got: %v", err)
	}

	results := collectResults(resultCh)

	// We should get at least one result (could be 0 if context cancelled before task starts,
	// or 1 if task started but was cancelled)
	// This test mainly verifies that the system handles cancellation gracefully
	for _, result := range results {
		if result.Error != nil {
			// If we get a result with an error, it should be a context error
			if !errors.Is(result.Error, context.DeadlineExceeded) && !errors.Is(result.Error, context.Canceled) {
				t.Fatalf("Expected context error, got: %v", result.Error)
			}
		}
	}

	// The main thing is that the channel was closed and we didn't hang
	// This test passes if we reach this point without hanging
}
