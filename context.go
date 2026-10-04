package banexg

import "context"

// ContextFromParams extracts the optional transport context and removes it
// from the parameter map before venue encoding.
func ContextFromParams(params map[string]interface{}) context.Context {
	if params == nil {
		return context.Background()
	}
	ctx, _ := params[ParamContext].(context.Context)
	delete(params, ParamContext)
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
