# Flutter Client Bug Fixes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 11 bugs in the Flutter client: 10 silent error catches and 1 hardcoded value, matching the web client fixes.

**Architecture:** Create a reusable snackbar helper for screen-level error display. Add `debugPrint` logging to service-level catches where returning null/false is correct behavior. Fix the hardcoded post_event_seconds with a slider in the add-rule dialog.

**Tech Stack:** Flutter/Dart, Riverpod, Dio, Material Design

---

## File Map

| File                                                              | Action | Responsibility                                                    |
| ----------------------------------------------------------------- | ------ | ----------------------------------------------------------------- |
| `clients/flutter/lib/utils/snackbar_helper.dart`                  | Create | Reusable error snackbar utility                                   |
| `clients/flutter/lib/services/auth_service.dart`                  | Modify | Add debugPrint logging to 3 catches                               |
| `clients/flutter/lib/services/playback_service.dart`              | Modify | Add debugPrint logging to listTimespans catch                     |
| `clients/flutter/lib/screens/cameras/recording_rules_screen.dart` | Modify | Fix silent catch in \_fetchStreams, add post_event_seconds slider |
| `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`   | Modify | Fix 2 silent catches                                              |
| `clients/flutter/lib/screens/settings/performance_panel.dart`     | Modify | Fix silent catch with error state                                 |
| `clients/flutter/lib/screens/screenshots/screenshots_screen.dart` | Modify | Fix silent catch in \_fetchCameras                                |
| `clients/flutter/lib/providers/settings_provider.dart`            | Modify | Remove catch in auditProvider, let Riverpod propagate             |

---

### Task 1: Create reusable snackbar helper

**Files:**

- Create: `clients/flutter/lib/utils/snackbar_helper.dart`

The codebase has ad-hoc `ScaffoldMessenger.of(context).showSnackBar(...)` calls scattered everywhere with inconsistent styling. Create a utility.

- [ ] **Step 1: Create the helper file**

Create `clients/flutter/lib/utils/snackbar_helper.dart`:

```dart
import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

void showErrorSnackBar(BuildContext context, String message) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      backgroundColor: NvrColors.bgSecondary,
      content: Text(
        message,
        style: const TextStyle(color: NvrColors.danger, fontSize: 13),
      ),
      behavior: SnackBarBehavior.floating,
      duration: const Duration(seconds: 4),
    ),
  );
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/utils/snackbar_helper.dart`
Expected: No issues.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/utils/snackbar_helper.dart
git commit -m "feat(flutter): add reusable snackbar helper utility"
```

---

### Task 2: Add logging to auth_service.dart silent catches

**Files:**

- Modify: `clients/flutter/lib/services/auth_service.dart`

The 3 catches in auth_service return meaningful values (false, null, empty) and callers handle them. Adding user-facing notifications here would be wrong — the caller is responsible for UX. But we need `debugPrint` logging for developer diagnostics.

- [ ] **Step 1: Add flutter foundation import**

At the top of `auth_service.dart`, add:

```dart
import 'package:flutter/foundation.dart';
```

- [ ] **Step 2: Fix validateServer catch (line 48-50)**

Replace:

```dart
  } catch (_) {
    return false;
  }
```

With:

```dart
  } catch (e) {
    debugPrint('[AuthService] validateServer failed: $e');
    return false;
  }
```

- [ ] **Step 3: Fix refresh catch (line 93-95)**

Replace:

```dart
  } catch (_) {
    return null;
  }
```

With:

```dart
  } catch (e) {
    debugPrint('[AuthService] token refresh failed: $e');
    return null;
  }
```

- [ ] **Step 4: Fix logout catch (line 107)**

Replace:

```dart
  } catch (_) {}
```

With:

```dart
  } catch (e) {
    debugPrint('[AuthService] logout revoke failed: $e');
  }
```

- [ ] **Step 5: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/services/auth_service.dart`
Expected: No issues.

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/services/auth_service.dart
git commit -m "fix(flutter): add debug logging to auth service error catches"
```

---

### Task 3: Add logging to playback_service.dart silent catch

**Files:**

- Modify: `clients/flutter/lib/services/playback_service.dart`

- [ ] **Step 1: Add foundation import**

Add at the top:

```dart
import 'package:flutter/foundation.dart';
```

- [ ] **Step 2: Fix listTimespans catch (line 60-62)**

Replace:

```dart
  } catch (e) {
    return [];
  }
```

With:

```dart
  } catch (e) {
    debugPrint('[PlaybackService] listTimespans failed: $e');
    return [];
  }
```

- [ ] **Step 3: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/services/playback_service.dart`
Expected: No issues.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/services/playback_service.dart
git commit -m "fix(flutter): add debug logging to playback service error catch"
```

---

### Task 4: Fix silent catch and add post_event_seconds in recording_rules_screen.dart

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/recording_rules_screen.dart`

Two bugs: (1) `_fetchStreams()` silently catches at line 44, (2) `post_event_seconds` hardcoded to 30 at line 319.

- [ ] **Step 1: Add snackbar_helper import**

Add after the existing imports:

```dart
import '../../utils/snackbar_helper.dart';
```

- [ ] **Step 2: Fix \_fetchStreams silent catch (line 44)**

Replace:

```dart
    } catch (_) {}
```

With:

```dart
    } catch (e) {
      if (mounted) showErrorSnackBar(context, 'Failed to load streams: $e');
    }
```

- [ ] **Step 3: Add postEventSeconds state to dialog**

In `_showAddDialog()` (line 127), add a new local variable after the existing ones (after line 132):

```dart
    int postEventSeconds = 30;
```

- [ ] **Step 4: Add slider to dialog UI**

In the dialog's Column children, after the mode dropdown (after line 207), add the slider conditionally shown when mode is 'motion':

```dart
                    if (selectedMode == 'motion') ...[
                      const SizedBox(height: 12),
                      const Text('Post-event buffer',
                          style: TextStyle(color: NvrColors.textSecondary, fontSize: 13)),
                      const SizedBox(height: 6),
                      Row(
                        children: [
                          Expanded(
                            child: Slider(
                              value: postEventSeconds.toDouble(),
                              min: 0,
                              max: 300,
                              divisions: 60,
                              activeColor: NvrColors.accent,
                              label: '${postEventSeconds}s',
                              onChanged: (v) => setDlgState(() => postEventSeconds = v.round()),
                            ),
                          ),
                          SizedBox(
                            width: 44,
                            child: Text(
                              '${postEventSeconds}s',
                              style: const TextStyle(color: NvrColors.textPrimary, fontSize: 13),
                              textAlign: TextAlign.end,
                            ),
                          ),
                        ],
                      ),
                    ],
```

- [ ] **Step 5: Pass postEventSeconds to \_saveNewRule**

In the `_showAddDialog()` save button's `onPressed` (around line 270-282), add a `postEventSeconds` parameter:

Replace the `_saveNewRule` call:

```dart
                    await _saveNewRule(
                      mode: selectedMode,
                      streamId: selectedStreamId,
                      startTime: selectedMode == 'schedule'
                          ? '${startTime.hour.toString().padLeft(2, '0')}:${startTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      endTime: selectedMode == 'schedule'
                          ? '${endTime.hour.toString().padLeft(2, '0')}:${endTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      daysOfWeek: selectedMode == 'schedule' ? selectedDays : null,
                    );
```

With:

```dart
                    await _saveNewRule(
                      mode: selectedMode,
                      streamId: selectedStreamId,
                      startTime: selectedMode == 'schedule'
                          ? '${startTime.hour.toString().padLeft(2, '0')}:${startTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      endTime: selectedMode == 'schedule'
                          ? '${endTime.hour.toString().padLeft(2, '0')}:${endTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      daysOfWeek: selectedMode == 'schedule' ? selectedDays : null,
                      postEventSeconds: postEventSeconds,
                    );
```

- [ ] **Step 6: Update \_saveNewRule signature and usage**

Update the method signature (line 294) to accept `postEventSeconds`:

Replace:

```dart
  Future<void> _saveNewRule({
    required String mode,
    String streamId = '',
    String? startTime,
    String? endTime,
    List<int>? daysOfWeek,
  }) async {
```

With:

```dart
  Future<void> _saveNewRule({
    required String mode,
    String streamId = '',
    String? startTime,
    String? endTime,
    List<int>? daysOfWeek,
    int postEventSeconds = 30,
  }) async {
```

Then replace line 319:

```dart
        if (backendMode == 'events') 'post_event_seconds': 30,
```

With:

```dart
        if (backendMode == 'events') 'post_event_seconds': postEventSeconds,
```

- [ ] **Step 7: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/recording_rules_screen.dart`
Expected: No issues.

- [ ] **Step 8: Commit**

```bash
git add clients/flutter/lib/screens/cameras/recording_rules_screen.dart
git commit -m "fix(flutter): add error feedback for stream fetch, expose post_event_seconds slider"
```

---

### Task 5: Fix silent catches in camera_detail_screen.dart

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

Two silent catches at lines 149 and 167 for schedule templates and recording rules fetches.

- [ ] **Step 1: Add snackbar_helper import**

Add after the existing imports:

```dart
import '../../utils/snackbar_helper.dart';
```

- [ ] **Step 2: Fix templates fetch catch (line 149)**

Replace:

```dart
      } catch (_) {}
```

With:

```dart
      } catch (e) {
        if (mounted) showErrorSnackBar(context, 'Failed to load schedule templates');
      }
```

- [ ] **Step 3: Fix recording rules fetch catch (line 167)**

Replace:

```dart
      } catch (_) {}
```

With:

```dart
      } catch (e) {
        if (mounted) showErrorSnackBar(context, 'Failed to load recording rules');
      }
```

- [ ] **Step 4: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/camera_detail_screen.dart`
Expected: No issues.

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "fix(flutter): add error feedback for template and rule fetch failures"
```

---

### Task 6: Fix silent catch in performance_panel.dart

**Files:**

- Modify: `clients/flutter/lib/screens/settings/performance_panel.dart`

The `_fetchMetrics` catch at line 50 sets `_loading = false` but shows no error.

- [ ] **Step 1: Add snackbar_helper import and error state**

Add import:

```dart
import '../../utils/snackbar_helper.dart';
```

Add a `_metricsError` field to the state class (after line 20):

```dart
  String? _metricsError;
```

- [ ] **Step 2: Fix \_fetchMetrics catch (lines 50-56)**

Replace:

```dart
    } catch (_) {
      if (mounted) {
        setState(() {
          _loading = false;
        });
      }
    }
```

With:

```dart
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _metricsError = 'Failed to load metrics';
        });
      }
    }
```

Also clear the error on successful fetch. In the success branch (inside the `if (data != null && mounted)` block), add `_metricsError = null;` to the setState:

Replace:

```dart
      if (data != null && mounted) {
        setState(() {
          _current = data['current'] as Map<String, dynamic>?;
          _history = (data['history'] as List<dynamic>?) ?? [];
          _loading = false;
        });
      }
```

With:

```dart
      if (data != null && mounted) {
        setState(() {
          _current = data['current'] as Map<String, dynamic>?;
          _history = (data['history'] as List<dynamic>?) ?? [];
          _loading = false;
          _metricsError = null;
        });
      }
```

- [ ] **Step 3: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/settings/performance_panel.dart`
Expected: No issues.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/settings/performance_panel.dart
git commit -m "fix(flutter): track error state in performance panel metrics fetch"
```

---

### Task 7: Fix silent catch in screenshots_screen.dart

**Files:**

- Modify: `clients/flutter/lib/screens/screenshots/screenshots_screen.dart`

Silent catch at line 39 in `_fetchCameras()`.

- [ ] **Step 1: Add snackbar_helper import**

Add after imports:

```dart
import '../../utils/snackbar_helper.dart';
```

- [ ] **Step 2: Fix \_fetchCameras catch (line 39)**

Replace:

```dart
    } catch (_) {}
```

With:

```dart
    } catch (e) {
      if (mounted) showErrorSnackBar(context, 'Failed to load cameras');
    }
```

- [ ] **Step 3: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/screenshots/screenshots_screen.dart`
Expected: No issues.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/screenshots/screenshots_screen.dart
git commit -m "fix(flutter): add error feedback for camera fetch in screenshots"
```

---

### Task 8: Fix silent catch in settings_provider.dart

**Files:**

- Modify: `clients/flutter/lib/providers/settings_provider.dart`

The `auditProvider` catches all errors and returns an empty list. Since this is a `FutureProvider`, Riverpod can propagate errors to the UI via `.error` state. Remove the try/catch.

- [ ] **Step 1: Remove the try/catch from auditProvider (lines 200-209)**

Replace:

```dart
final auditProvider = FutureProvider<List<AuditEntry>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/audit', queryParameters: {'limit': 100});
    return (res.data as List).map((e) => AuditEntry.fromJson(e as Map<String, dynamic>)).toList();
  } catch (_) {
    return [];
  }
});
```

With:

```dart
final auditProvider = FutureProvider<List<AuditEntry>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/audit', queryParameters: {'limit': 100});
  return (res.data as List).map((e) => AuditEntry.fromJson(e as Map<String, dynamic>)).toList();
});
```

- [ ] **Step 2: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/providers/settings_provider.dart`
Expected: No issues.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/providers/settings_provider.dart
git commit -m "fix(flutter): let auditProvider propagate errors to UI via Riverpod"
```

---

### Task 9: Final verification

- [ ] **Step 1: Run full Flutter analysis**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze`
Expected: No errors (warnings are acceptable).

- [ ] **Step 2: Verify no remaining silent catches in modified files**

Search for `catch (_)` in all modified files and verify each remaining one is intentional (e.g., WebSocket reconnect).

- [ ] **Step 3: Build check**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter build apk --debug 2>&1 | tail -5`
Expected: Build succeeds (or environment-specific issues only).
