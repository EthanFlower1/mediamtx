# KAI-111: Device Service SET Operations

## Summary

Add missing ONVIF Device Service SET operations to the NVR, exposing them via REST API with input validation. All operations follow the established pattern: package-level ONVIF functions + Gin HTTP handlers.

## Operations

| Function | Type | API Endpoint |
|----------|------|-------------|
| SetSystemDateAndTime | SET | PUT /cameras/:id/device/datetime |
| SetDNS | SET | PUT /cameras/:id/device/network/dns |
| SetNTP | SET | PUT /cameras/:id/device/network/ntp |
| SetNetworkInterfaces | SET | PUT /cameras/:id/device/network/interfaces/:token |
| GetNetworkDefaultGateway | GET | GET /cameras/:id/device/network/gateway |
| SetNetworkDefaultGateway | SET | PUT /cameras/:id/device/network/gateway |
| SetScopes | SET | PUT /cameras/:id/device/scopes |
| AddScopes | SET | POST /cameras/:id/device/scopes |
| RemoveScopes | SET | DELETE /cameras/:id/device/scopes |
| GetDiscoveryMode | GET | GET /cameras/:id/device/discovery-mode |
| SetDiscoveryMode | SET | PUT /cameras/:id/device/discovery-mode |
| GetSystemLog | GET | GET /cameras/:id/device/system-log |
| GetSystemSupportInformation | GET | GET /cameras/:id/device/support-info |

## Files Modified

- `internal/nvr/onvif/device_mgmt.go` — New ONVIF wrapper functions + request/response types
- `internal/nvr/api/cameras.go` — New Gin handler methods on CameraHandler
- `internal/nvr/api/router.go` — New route registrations

## Validation Rules

- **SetSystemDateAndTime**: type must be "NTP" or "Manual"; timezone required; date/time fields required when Manual
- **SetDNS/SetNTP**: validate IP address format for manual servers
- **SetNetworkInterfaces**: validate IP address and prefix length (1-32) for manual IPv4
- **SetNetworkDefaultGateway**: validate gateway IP format
- **SetScopes/AddScopes**: non-empty scope URI list required
- **RemoveScopes**: non-empty scope URI list required
- **SetDiscoveryMode**: must be "Discoverable" or "NonDiscoverable"

## Request/Response Types

### SetDateTimeRequest
```json
{"type": "NTP|Manual", "daylight_saving": false, "timezone": "UTC-5", "utc_date_time": {"year":2026,"month":4,"day":3,"hour":12,"minute":0,"second":0}}
```

### SetDNSRequest / SetNTPRequest
```json
{"from_dhcp": false, "servers": ["8.8.8.8", "8.8.4.4"]}
```

### SetNetworkInterfaceRequest
```json
{"enabled": true, "ipv4": {"enabled": true, "dhcp": false, "address": "192.168.1.100", "prefix_length": 24}}
```
Response includes `{"reboot_needed": true}` from the device.

### GatewayInfo
```json
{"ipv4": ["192.168.1.1"], "ipv6": []}
```

### ScopesRequest
```json
{"scopes": ["onvif://www.onvif.org/name/MyCamera"]}
```

### DiscoveryModeInfo
```json
{"mode": "Discoverable"}
```

### SystemLogInfo / SupportInfo
```json
{"content": "...log text..."}
```

SystemLog accepts query param `?type=System` (default) or `?type=Access`.
