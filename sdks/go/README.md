# KaiVue Go SDK

Official Go SDK for the KaiVue VMS public API. Zero external dependencies -- uses only the Go standard library.

## Installation

```bash
go get github.com/kaivue/sdk-go@latest
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/kaivue/sdk-go/kaivue"
)

func main() {
    client := kaivue.NewClient("https://your-instance.kaivue.io",
        kaivue.WithAPIKey("your-api-key"),
    )

    ctx := context.Background()

    // List cameras
    resp, err := client.Cameras.List(ctx, &kaivue.ListCamerasRequest{})
    if err != nil {
        log.Fatal(err)
    }
    for _, cam := range resp.Cameras {
        fmt.Printf("%s: %s [%s]\n", cam.ID, cam.Name, cam.State)
    }

    // Create a camera
    cam, err := client.Cameras.Create(ctx, &kaivue.CreateCameraRequest{
        Name:       "Front Door",
        IPAddress:  "192.168.1.10",
        RecorderID: "rec-01",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created: %s\n", cam.ID)
}
```

## Authentication

### API Key

```go
client := kaivue.NewClient(url, kaivue.WithAPIKey("your-key"))
```

### OAuth Bearer Token

```go
client := kaivue.NewClient(url, kaivue.WithOAuth("your-token"))
```

### Custom Auth

```go
client := kaivue.NewClient(url, kaivue.WithAuth(&MyCustomAuth{}))
```

## Services

| Service | Description |
|---------|-------------|
| `client.Cameras` | Camera CRUD, filtering by state/recorder |
| `client.Users` | User management |
| `client.Recordings` | Recording search, export, deletion |
| `client.Events` | AI/motion event queries, acknowledgment |
| `client.Schedules` | Recording schedule management |
| `client.Retention` | Retention policy CRUD + apply to cameras |
| `client.Integrations` | Webhook/MQTT/syslog integration management |

## Error Handling

```go
cam, err := client.Cameras.Get(ctx, "nonexistent")
if kaivue.IsNotFound(err) {
    fmt.Println("Camera not found")
} else if kaivue.IsAuthError(err) {
    fmt.Println("Check your API key")
} else if kaivue.IsValidationError(err) {
    apiErr := err.(*kaivue.APIError)
    for _, fe := range apiErr.FieldErrors {
        fmt.Printf("  %s: %s\n", fe.Field, fe.Message)
    }
}
```

## Version Compatibility

| SDK Version | API Version |
|-------------|-------------|
| 0.1.x | v1 |

## License

Apache-2.0
