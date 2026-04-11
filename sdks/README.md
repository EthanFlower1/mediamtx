# KaiVue Public SDKs

Official SDKs for the KaiVue VMS public API, covering 7 services and 33 methods.

## Available SDKs

| Language | Package | Directory |
|----------|---------|-----------|
| Python | `kaivue` (PyPI) | [python/](python/) |
| Go | `github.com/kaivue/sdk-go` | [go/](go/) |
| TypeScript | `@kaivue/sdk` (npm) | [typescript/](typescript/) |

Java and C# SDKs are planned for v1.x.

## API Services

All SDKs provide access to these 7 services:

| Service | Methods | Description |
|---------|---------|-------------|
| CameraService | 5 | CRUD + list with filters |
| UserService | 5 | CRUD + list with search |
| RecordingService | 4 | Get, list, delete, export |
| EventService | 3 | Get, list, acknowledge |
| ScheduleService | 5 | CRUD + list |
| RetentionService | 6 | CRUD + list + apply to cameras |
| IntegrationService | 6 | CRUD + list + test connectivity |
| **Total** | **33** | |

## Authentication

All SDKs support two authentication methods:

1. **API Key** -- static key sent via `X-API-Key` header
2. **OAuth2 Bearer** -- token sent via `Authorization: Bearer <token>` header, with optional auto-refresh

## Proto Definition

The public API is defined in [`proto/kaivue/v1/public/public_api.proto`](proto/kaivue/v1/public/public_api.proto).

## Building and Testing

```bash
# Build all SDKs
make build

# Run all tests
make test

# Lint proto files
make lint
```

## Version Compatibility

| SDK Version | API Version | Status |
|-------------|-------------|--------|
| 0.1.x | v1 | Current |

## License

Apache-2.0
