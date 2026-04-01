# NVR Client Integration Tests

End-to-end integration tests that exercise the full Flutter NVR client against a running MediaMTX backend.

## Prerequisites

1. **Running NVR backend** — MediaMTX server must be running and reachable (default: `http://localhost:9997`).
2. **At least one camera configured** — The server should have at least one camera added and ideally connected/online so stat tiles show real data.
3. **Valid credentials** — Default test credentials are `admin` / `admin`.

## Test files

| File | Description |
|---|---|
| `app_test.dart` | Original integration tests covering auth, live view, camera management, playback, and navigation. |
| `nvr_e2e_test.dart` | Comprehensive E2E tests covering all major screens: server setup, login, live view grid changes, camera detail with live data, settings system info, search, and camera panel. |

## Running the tests

### Default settings (localhost:9997, admin/admin)

```bash
cd clients/flutter
flutter test integration_test/nvr_e2e_test.dart -d macos
```

### Custom server and credentials

```bash
flutter test integration_test/nvr_e2e_test.dart -d macos \
  --dart-define=NVR_SERVER_URL=http://192.168.1.100:9997 \
  --dart-define=NVR_USERNAME=admin \
  --dart-define=NVR_PASSWORD=mypassword
```

### Run on iOS Simulator

```bash
flutter test integration_test/nvr_e2e_test.dart -d <simulator-id>
```

### Run a single test group

```bash
flutter test integration_test/nvr_e2e_test.dart -d macos --name "Auth flow"
flutter test integration_test/nvr_e2e_test.dart -d macos --name "Live view"
flutter test integration_test/nvr_e2e_test.dart -d macos --name "Camera detail"
```

## What each test verifies

### Test 1: Server setup and login flow (`Auth flow`)
- App starts on the server setup screen when no server is configured
- SERVER URL text field is present and accepts input
- CONNECT button triggers navigation to login screen
- USERNAME and PASSWORD fields are present
- SIGN IN button authenticates and navigates to Live View

### Test 2: Live view shows grid (`Live view > live view shows grid`)
- "Live View" title is displayed
- "ALL CAMERAS" badge is present
- Grid size selector (segmented control) shows 1x1, 2x2, etc.
- GridView widget is rendered with camera tiles or empty slots

### Test 3: Grid size changes (`Live view > can change grid size`)
- Tapping 1x1 sets grid crossAxisCount to 1
- Tapping 3x3 sets grid crossAxisCount to 3
- Tapping 4x4 sets grid crossAxisCount to 4

### Test 4: Camera detail with live data (`Camera detail > can navigate to camera detail`)
- Navigates from Live View to Devices tab
- Devices page title is visible
- At least one device card exists
- Tapping a card opens the camera detail screen
- Detail screen shows UPTIME, STORAGE, EVENTS TODAY, and RETENTION stat tiles
- RECORDING and AI DETECTION sections are present

### Test 5: Camera detail live preview (`Camera detail > camera detail shows live preview`)
- Camera detail has an AspectRatio widget for the 16:9 video preview
- No loading spinner remains (data finished loading)

### Test 6: Settings system info (`Settings`)
- SETTINGS page title is visible
- VERSION, UPTIME, and CAMERAS stat tiles are shown
- SYSTEM INFO and CONNECTION sections are present

### Test 7: Search screen (`Search`)
- "Search" page title is visible
- Search text field and SEARCH button exist
- Empty state message shown before any search
- After typing "person" and tapping SEARCH, either results or "No results found" appears

### Test 8: Camera panel (`Camera panel`)
- Desktop only (>= 600px width)
- Tapping the active Live nav icon toggles the camera panel
- Panel shows "CAMERAS" header and "Search cameras..." field
- Close (X) button dismisses the panel

### Bonus: Navigation round-trip (`Navigation`)
- All five tabs (Live, Playback, Search, Devices, Settings) are reachable
- Each tab renders its expected content

## Troubleshooting

- **Tests hang on pumpAndSettle**: The server may be unreachable or slow. Increase timeout durations or check network connectivity.
- **"Server unreachable" error**: Verify the server URL is correct and the backend is running.
- **Auth failures**: Check that the username/password match the server configuration.
- **Camera panel test skipped**: This test only runs when the window width is >= 600px. On narrow simulators it is silently skipped.
- **GridView assertions fail**: Make sure at least one camera is configured so the grid renders camera tiles rather than only empty slots.
