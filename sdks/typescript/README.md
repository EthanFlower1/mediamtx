# KaiVue TypeScript SDK

Official TypeScript SDK for the KaiVue VMS public API. Manage cameras, users, recordings, events, schedules, retention policies, and integrations.

## Installation

```bash
npm install @kaivue/sdk
```

## Quick Start

```typescript
import { KaiVueClient } from "@kaivue/sdk";

const client = new KaiVueClient("https://your-instance.kaivue.io", {
  apiKey: "your-api-key",
});

// List all cameras
const { cameras } = await client.cameras.list();
for (const cam of cameras) {
  console.log(`${cam.name}: ${cam.state}`);
}

// Create a camera
const camera = await client.cameras.create({
  name: "Front Door",
  ip_address: "192.168.1.10",
  recorder_id: "rec-01",
});

// Get recent events
const { events } = await client.events.list({ camera_id: camera.id });

// Auto-paginate through all recordings
for await (const rec of client.recordings.listAll({ camera_id: camera.id })) {
  console.log(`${rec.start_time} - ${rec.end_time}`);
}
```

## Authentication

### API Key

```typescript
const client = new KaiVueClient("https://...", { apiKey: "your-key" });
```

### OAuth2

```typescript
import { KaiVueClient, OAuthAuth } from "@kaivue/sdk";

const auth = new OAuthAuth({
  clientId: "your-client-id",
  clientSecret: "your-client-secret",
  tokenUrl: "https://your-instance.kaivue.io/oauth/token",
});
const client = new KaiVueClient("https://...", { auth });
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

```typescript
import { NotFoundError, AuthenticationError, ValidationError } from "@kaivue/sdk";

try {
  const camera = await client.cameras.get("nonexistent");
} catch (e) {
  if (e instanceof NotFoundError) {
    console.log(`Not found: ${e.message} (request_id=${e.requestId})`);
  } else if (e instanceof AuthenticationError) {
    console.log("Check your API key");
  } else if (e instanceof ValidationError) {
    for (const fe of e.fieldErrors) {
      console.log(`  ${fe.field}: ${fe.message}`);
    }
  }
}
```

## Version Compatibility

| SDK Version | API Version |
|-------------|-------------|
| 0.1.x | v1 |

## License

Apache-2.0
