package onvif

import "errors"

// ErrScanInProgress is returned when a scan is requested while one is already running.
var ErrScanInProgress = errors.New("scan already in progress")
