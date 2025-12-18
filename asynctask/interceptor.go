package asynctask

import "context"

// Interceptor is a function that can wrap the task execution.
// It allows injecting logic before and after the task runs, or handling errors.
type Interceptor[Params, Progress, Result any] func(
	ctx context.Context,
	params Params,
	reporter ProgressReporter[Progress],
	next Func[Params, Progress, Result],
) (Result, error)

// chainInterceptors chains multiple interceptors into a single one.
func chainInterceptors[Params, Progress, Result any](
	interceptors []Interceptor[Params, Progress, Result],
) Interceptor[Params, Progress, Result] {
	if len(interceptors) == 0 {
		return nil
	}

	return func(
		ctx context.Context,
		params Params,
		reporter ProgressReporter[Progress],
		last Func[Params, Progress, Result],
	) (Result, error) {
		chain := last
		// Wrap in reverse order so the first interceptor is the outer-most.
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(c context.Context, p Params, r ProgressReporter[Progress]) (Result, error) {
				return interceptor(c, p, r, next)
			}
		}
		return chain(ctx, params, reporter)
	}
}
