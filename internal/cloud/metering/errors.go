package metering

import "errors"

// Sentinel errors. Callers use errors.Is to distinguish them.
var (
	// ErrMissingTenant is returned when Record or any query is called
	// without a non-empty TenantID. Fail-closed to preserve Seam #4.
	ErrMissingTenant = errors.New("metering: missing tenant_id")

	// ErrUnknownMetric is returned when Record is called with a metric
	// name that is not in AllMetrics. The metric list is closed in v1 so
	// billing rules stay in sync with the emitted vocabulary.
	ErrUnknownMetric = errors.New("metering: unknown metric")

	// ErrNegativeValue is returned when Record is called with a value
	// less than zero. All v1 metrics are monotonic and non-negative.
	ErrNegativeValue = errors.New("metering: value must be non-negative")

	// ErrInvalidPeriod is returned when Aggregator.Run is called with a
	// period where end <= start.
	ErrInvalidPeriod = errors.New("metering: invalid period (end must be after start)")

	// ErrUnknownDialect is returned when the Store is constructed with a
	// Dialect value not recognised by this package.
	ErrUnknownDialect = errors.New("metering: unknown dialect")
)
