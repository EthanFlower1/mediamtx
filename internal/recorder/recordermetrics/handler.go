package recordermetrics

// Handler is defined in metrics.go as a method on *Metrics.
// This file exists as a named home for HTTP handler concerns should
// they grow (e.g. health probes, additional middleware) in future phases.
//
// Usage in boot.go:
//
//	router.GET("/metrics", gin.WrapH(metrics.Handler()))
