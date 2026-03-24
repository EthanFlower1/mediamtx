# Flutter Client Issues

Bugs and issues found during testing of the macOS Flutter client.

---

1. **AI overlay not visible in live view** — Bounding boxes from AI detections don't appear over the camera feed in the live view grid or fullscreen view.
2. **Clip Search crashes: `type 'int' is not a subtype of type 'String' in type cast`** — Search results parsing fails, likely a JSON field being cast as String when it's actually an int (e.g., `detection_id` or `event_id`).
3. **Camera settings only show the selected RTSP URL, not all available profiles** — When viewing camera details, only the RTSP URL chosen during add is shown. Should display all ONVIF media profiles so the user can switch between main/sub streams.
4. **Zones tab crashes: `Type 'String' is not a subtype of type 'List<dynamic>?' in type cast`** — Zone editor fails to parse zone data. The `polygon` field is likely returned as a JSON string from the API but the model expects a `List<List<double>>`.
5. **Playback page has no real timeline** — Only basic VCR controls (play/pause/skip). Missing a full visual timeline showing recording segments and motion events like Milestone or Frigate have. Should be a scrubable bar showing where recordings exist, with event markers and a draggable playhead.

