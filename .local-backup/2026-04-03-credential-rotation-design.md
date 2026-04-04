# ONVIF Credential Rotation Design

**Date:** 2026-04-03
**Ticket:** KAI-23

## Summary

Add a dedicated API endpoint for rotating ONVIF camera credentials with validation. New credentials are tested against the camera's ONVIF endpoint before being saved. If validation fails, old credentials remain untouched.

## Endpoint

`POST /api/v1/cameras/:id/rotate-credentials`

### Request

```json
{
  "username": "newuser",
  "password": "newpass"
}
```

At least one of `username` or `password` must be provided.

### Success Response (200)

```json
{
  "id": "camera-uuid",
  "message": "credentials rotated successfully"
}
```

### Error Responses

- **400** — Camera has no ONVIF endpoint, missing credentials in request, or new credentials failed ONVIF authentication
- **404** — Camera not found
- **500** — Internal error (DB save failure after successful validation)

```json
{
  "error": "new credentials failed ONVIF authentication: connection refused"
}
```

## Flow

1. Parse and validate the request body
2. Fetch camera from DB by ID; return 404 if not found
3. Validate the camera has an ONVIF endpoint configured; return 400 if not
4. Determine effective credentials: use new values where provided, fall back to existing (decrypted) values for omitted fields
5. Test effective credentials via `onvif.ProbeDeviceFull(endpoint, username, password)`
6. If probe fails: return 400 with the error detail; old credentials remain in DB
7. If probe succeeds: encrypt new password, update camera record in DB
8. Log audit event for credential rotation
9. If the scheduler has active event subscriptions for this camera, reinitialize them with the new credentials
10. Return 200 success

## Rollback

Rollback is implicit: credentials are only written to the DB after ONVIF validation succeeds. If validation fails, nothing changes. If the DB write fails after validation, the old credentials remain (the DB is the source of truth).

## Code Changes

### `internal/nvr/api/cameras.go`

Add `RotateCredentials` method to `CameraHandler`:

- Binds request JSON
- Fetches camera, validates ONVIF endpoint exists
- Decrypts existing password to use as fallback
- Calls `onvif.ProbeDeviceFull()` with new credentials
- On success: encrypts new password, calls `DB.UpdateCamera()`
- Logs audit event via `h.Audit`

### `internal/nvr/api/router.go`

Register the new route:

```go
protected.POST("/cameras/:id/rotate-credentials", cameraHandler.RotateCredentials)
```

### `internal/nvr/scheduler/scheduler.go`

Add `ResubscribeCamera(cameraID string)` method:

- Tears down existing event subscription for the camera
- Fetches fresh camera record from DB
- Decrypts new credentials
- Creates new event subscription

### Audit Logging

Log credential rotation as action `"credential_rotation"` with camera ID and name. Do not log the credentials themselves.

## Not In Scope

- Changing the password on the camera itself via ONVIF `SetUser`
- Bulk credential rotation across multiple cameras
- Password complexity or policy enforcement
- Credential rotation scheduling
