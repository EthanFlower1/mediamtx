package api

import "log"

// Structured logging helpers for the NVR API.
// All log lines use the format: [NVR] [LEVEL] message
// This provides consistent, grep-friendly output while keeping the
// implementation simple (stdlib log).

// nvrLogInfo logs an informational message with the NVR prefix.
func nvrLogInfo(component, msg string) {
	log.Printf("[NVR] [INFO] [%s] %s", component, msg)
}

// nvrLogWarn logs a warning message with the NVR prefix.
func nvrLogWarn(component, msg string) {
	log.Printf("[NVR] [WARN] [%s] %s", component, msg)
}

// nvrLogError logs an error message with the NVR prefix.
func nvrLogError(component, msg string, err error) {
	log.Printf("[NVR] [ERROR] [%s] %s: %v", component, msg, err)
}
