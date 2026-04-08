# `internal/recorder/state` — Recorder local cache

The Recorder-side SQLite cache that mirrors the cameras assigned to
this Recorder from the Directory. **This is the source of truth for
runtime behavior**: the Recorder reads from this cache — not from the
Directory — when deciding what to capture. If the Directory becomes
unreachable, the Recorder keeps running against the last-known-good
snapshot stored here.

Driven by `modernc.org/sqlite` (pure Go, no CGO — per `CLAUDE.md`).

## Schema

```
+------------------------------+        +--------------------------+
| assigned_cameras             |        | segment_index            |
+------------------------------+        +--------------------------+
| camera_id           TEXT PK  |------->| camera_id  TEXT (PK pt1) |
| config              JSON     |        | start_ts   TEXT (PK pt2) |
| config_version      INTEGER  |        | end_ts     TEXT          |
| rtsp_credentials    BLOB     |        | path       TEXT          |
| assigned_at         TEXT     |        | size_bytes INTEGER       |
| updated_at          TEXT     |        | uploaded_to_cloud_archive|
| last_state_push_at  TEXT     |        +--------------------------+
+------------------------------+
                                        +--------------------------+
                                        | local_state              |
                                        +--------------------------+
                                        | key   TEXT PK            |
                                        | value JSON               |
                                        +--------------------------+
```

- **`assigned_cameras`** — one row per camera the Directory has
  assigned to this Recorder. `config` is a JSON blob of
  `state.CameraConfig`. `rtsp_credentials` is an opaque ciphertext
  produced by the injected `Cryptostore` (real impl from
  `internal/shared/cryptostore`, KAI-251). **Plaintext secrets are
  never stored.**
- **`local_state`** — free-form JSON key/value store for Recorder-local
  flags: last successful Directory sync, pending-reboot markers, etc.
- **`segment_index`** — thin index over on-disk recorded segments.
  Written by the capture pipeline as segments close, read by playback
  queries and the cloud-archive uploader.

### Migrations

Migrations are ordered, additive, and **frozen once shipped**. To
introduce a schema change, append a new entry to `migrations.go` — do
**not** edit any past migration. Each migration runs inside its own
transaction and is recorded in the `schema_migrations` table.

## Usage

```go
import (
    "context"
    "time"

    "github.com/bluenviron/mediamtx/internal/recorder/state"
    // "github.com/bluenviron/mediamtx/internal/shared/cryptostore" // KAI-251
)

func example(ctx context.Context) error {
    // crypto := cryptostore.New(...) // KAI-251 real impl
    store, err := state.Open("/var/lib/kaivue/recorder/state.db", state.Options{
        Cryptostore: state.NoopCryptostore{}, // swap for real impl in prod
    })
    if err != nil {
        return err
    }
    defer store.Close()

    // Reconcile the Directory-pushed snapshot into the cache.
    diff, err := store.ReconcileAssignments(ctx, []state.AssignedCamera{
        {
            CameraID:      "cam-front-door",
            ConfigVersion: 7,
            RTSPPassword:  "s3cret",
            Config: state.CameraConfig{
                ID:            "cam-front-door",
                Name:          "Front Door",
                RTSPURL:       "rtsp://192.168.1.10/stream1",
                RTSPUsername:  "admin",
                RetentionDays: 14,
                Tags:          []string{"outdoor", "entry"},
            },
        },
    })
    if err != nil {
        return err
    }
    _ = diff // diff.Added / Updated / Removed / Unchanged

    // Drive capture from the cache — never from the Directory directly.
    cams, err := store.ListAssigned(ctx)
    if err != nil {
        return err
    }
    for _, cam := range cams {
        _ = cam.Config.RTSPURL
        _ = cam.RTSPPassword // decrypted by the Store at read time
    }

    // Record a segment as it closes.
    if err := store.AppendSegment(ctx, state.Segment{
        CameraID: "cam-front-door",
        StartTS:  time.Now().UTC().Add(-1 * time.Minute),
        EndTS:    time.Now().UTC(),
        Path:     "/var/lib/kaivue/recorder/segments/cam-front-door/000001.mp4",
        SizeBytes: 4_500_000,
    }); err != nil {
        return err
    }

    // Playback window query.
    segs, err := store.QuerySegments(ctx,
        "cam-front-door",
        time.Now().Add(-1*time.Hour), time.Now())
    if err != nil {
        return err
    }
    _ = segs

    // Local KV.
    _ = store.SetState(ctx, "last_directory_sync", time.Now().UTC())
    return nil
}
```

## Cryptostore

The package declares its own minimal `Cryptostore` interface:

```go
type Cryptostore interface {
    Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
    Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}
```

This is a strict subset of the interface `internal/shared/cryptostore`
(KAI-251) will expose. At runtime, production Recorders inject the real
implementation into `state.Options{Cryptostore: ...}`. Tests use a
deterministic stub (see `store_test.go`). A `NoopCryptostore`
passthrough is provided for local development and is the default when
`Options.Cryptostore` is nil — **do not use it in production**.

## Tests

```
go test ./internal/recorder/state/...
```

Covers: migration application, upsert/get/list/remove round-trip,
reconcile diff semantics, segment append + time-range query, JSON
config stability, local state K/V, and cryptostore ciphertext-at-rest.
