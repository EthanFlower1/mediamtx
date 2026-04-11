# KaiVue Python SDK

Official Python SDK for the KaiVue VMS public API. Manage cameras, users, recordings, events, schedules, retention policies, and integrations.

## Installation

```bash
pip install kaivue
```

## Quick Start

```python
from kaivue import KaiVueClient

client = KaiVueClient("https://your-instance.kaivue.io", api_key="your-api-key")

# List all cameras
cameras = client.cameras.list()
for cam in cameras:
    print(f"{cam.name}: {cam.state.value}")

# Create a camera
camera = client.cameras.create(
    name="Front Door",
    ip_address="192.168.1.10",
    recorder_id="rec-01",
)

# Get recent events
events = client.events.list(camera_id=camera.id)

# Auto-paginate through all recordings
for recording in client.recordings.list_all(camera_id=camera.id):
    print(f"{recording.start_time} - {recording.end_time}")
```

## Authentication

### API Key

```python
client = KaiVueClient("https://...", api_key="your-key")
```

### OAuth2

```python
from kaivue import KaiVueClient, OAuthAuth

auth = OAuthAuth(
    client_id="your-client-id",
    client_secret="your-client-secret",
    token_url="https://your-instance.kaivue.io/oauth/token",
)
client = KaiVueClient("https://...", auth=auth)
```

## Services

| Service | Description |
|---------|-------------|
| `client.cameras` | Camera CRUD, filtering by state/recorder |
| `client.users` | User management |
| `client.recordings` | Recording search, export, deletion |
| `client.events` | AI/motion event queries, acknowledgment |
| `client.schedules` | Recording schedule management |
| `client.retention` | Retention policy CRUD + apply to cameras |
| `client.integrations` | Webhook/MQTT/syslog integration management |

## Error Handling

```python
from kaivue.errors import NotFoundError, AuthenticationError, ValidationError

try:
    camera = client.cameras.get("nonexistent")
except NotFoundError as e:
    print(f"Not found: {e.message} (request_id={e.request_id})")
except AuthenticationError:
    print("Check your API key")
except ValidationError as e:
    for fe in e.field_errors:
        print(f"  {fe.field}: {fe.message}")
```

## Async Support

```python
from kaivue.client import AsyncKaiVueClient

async with AsyncKaiVueClient("https://...", api_key="key") as client:
    data = await client.get("/v1/cameras")
```

## Version Compatibility

| SDK Version | API Version |
|-------------|-------------|
| 0.1.x | v1 |

## License

Apache-2.0
