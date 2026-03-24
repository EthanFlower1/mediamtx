# Flutter NVR Client — Plan 4: Management + Settings

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add camera management (CRUD, ONVIF discovery, zone editor, recording rules), user management, and settings (system info, storage, backups, audit) to complete the Flutter NVR client.

**Architecture:** Camera management uses tabbed detail views. Zone editor uses CustomPainter + GestureDetector for polygon drawing on camera snapshots. Settings uses a tabbed layout matching the web UI. All screens use existing ApiClient + Riverpod providers.

**Tech Stack:** Flutter, Riverpod, CustomPainter, dio

**Spec:** `docs/superpowers/specs/2026-03-23-flutter-client-design.md` (Sections 8, 9)

**Prerequisite:** Plans 1-3 must be complete. The app has: auth, navigation, live view, playback, search.

---

## File Structure

| File | Task | Purpose |
|------|------|---------|
| `clients/flutter/lib/models/zone.dart` | 1 | Zone + alert rule models |
| `clients/flutter/lib/models/recording_rule.dart` | 1 | Recording rule model |
| `clients/flutter/lib/providers/settings_provider.dart` | 1 | System info, storage, audit providers |
| `clients/flutter/lib/screens/cameras/camera_list_screen.dart` | 2 | Camera list with status + actions |
| `clients/flutter/lib/screens/cameras/add_camera_screen.dart` | 2 | ONVIF discovery + manual add |
| `clients/flutter/lib/screens/cameras/camera_detail_screen.dart` | 3 | Tabbed camera config |
| `clients/flutter/lib/screens/cameras/recording_rules_screen.dart` | 3 | Recording rules CRUD |
| `clients/flutter/lib/screens/cameras/zone_editor_screen.dart` | 4 | Polygon drawing on snapshot |
| `clients/flutter/lib/screens/settings/settings_screen.dart` | 5 | Tabbed settings |
| `clients/flutter/lib/screens/settings/storage_panel.dart` | 5 | Disk usage + per-camera |
| `clients/flutter/lib/screens/settings/user_management_screen.dart` | 6 | User CRUD + password change |
| `clients/flutter/lib/screens/settings/backup_panel.dart` | 5 | Backup create/list/download |
| `clients/flutter/lib/screens/settings/audit_panel.dart` | 5 | Audit log + CSV export |
| `clients/flutter/lib/widgets/camera_status_badge.dart` | 2 | Reusable online/offline badge |

---

### Task 1: Models + Settings Provider

**Files:**
- Create: `clients/flutter/lib/models/zone.dart`
- Create: `clients/flutter/lib/models/recording_rule.dart`
- Create: `clients/flutter/lib/providers/settings_provider.dart`

- [ ] **Step 1: Create zone model**

```dart
// clients/flutter/lib/models/zone.dart

class AlertRule {
  final int? id;
  final int? zoneId;
  final String className;
  final bool enabled;
  final int cooldownSeconds;
  final int loiterSeconds;
  final bool notifyOnEnter;
  final bool notifyOnLeave;
  final bool notifyOnLoiter;

  AlertRule({
    this.id,
    this.zoneId,
    required this.className,
    this.enabled = true,
    this.cooldownSeconds = 30,
    this.loiterSeconds = 0,
    this.notifyOnEnter = true,
    this.notifyOnLeave = false,
    this.notifyOnLoiter = false,
  });

  factory AlertRule.fromJson(Map<String, dynamic> json) => AlertRule(
    id: json['id'] as int?,
    zoneId: json['zone_id'] as int?,
    className: json['class_name'] as String? ?? '',
    enabled: json['enabled'] as bool? ?? true,
    cooldownSeconds: json['cooldown_seconds'] as int? ?? 30,
    loiterSeconds: json['loiter_seconds'] as int? ?? 0,
    notifyOnEnter: json['notify_on_enter'] as bool? ?? true,
    notifyOnLeave: json['notify_on_leave'] as bool? ?? false,
    notifyOnLoiter: json['notify_on_loiter'] as bool? ?? false,
  );

  Map<String, dynamic> toJson() => {
    'class_name': className,
    'enabled': enabled,
    'cooldown_seconds': cooldownSeconds,
    'loiter_seconds': loiterSeconds,
    'notify_on_enter': notifyOnEnter,
    'notify_on_leave': notifyOnLeave,
    'notify_on_loiter': notifyOnLoiter,
  };
}

class DetectionZone {
  final int? id;
  final String cameraId;
  final String name;
  final List<List<double>> polygon; // [[x1,y1],[x2,y2],...]
  final bool enabled;
  final List<AlertRule> rules;

  DetectionZone({
    this.id,
    required this.cameraId,
    required this.name,
    required this.polygon,
    this.enabled = true,
    this.rules = const [],
  });

  factory DetectionZone.fromJson(Map<String, dynamic> json) {
    final poly = (json['polygon'] as List?)
        ?.map((p) => (p as List).map((v) => (v as num).toDouble()).toList())
        .toList() ?? [];
    return DetectionZone(
      id: json['id'] as int?,
      cameraId: json['camera_id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      polygon: poly,
      enabled: json['enabled'] as bool? ?? true,
      rules: (json['rules'] as List?)?.map((r) => AlertRule.fromJson(r as Map<String, dynamic>)).toList() ?? [],
    );
  }

  Map<String, dynamic> toJson() => {
    'name': name,
    'polygon': polygon,
    'enabled': enabled,
    'rules': rules.map((r) => r.toJson()).toList(),
  };
}
```

- [ ] **Step 2: Create recording rule model**

```dart
// clients/flutter/lib/models/recording_rule.dart

class RecordingRule {
  final int? id;
  final String cameraId;
  final String mode;      // "continuous", "motion", "schedule"
  final String? startTime; // "HH:MM" for schedule mode
  final String? endTime;
  final List<int>? daysOfWeek; // 0=Sun..6=Sat
  final bool enabled;

  RecordingRule({
    this.id,
    required this.cameraId,
    required this.mode,
    this.startTime,
    this.endTime,
    this.daysOfWeek,
    this.enabled = true,
  });

  factory RecordingRule.fromJson(Map<String, dynamic> json) => RecordingRule(
    id: json['id'] as int?,
    cameraId: json['camera_id'] as String? ?? '',
    mode: json['mode'] as String? ?? 'continuous',
    startTime: json['start_time'] as String?,
    endTime: json['end_time'] as String?,
    daysOfWeek: (json['days_of_week'] as List?)?.cast<int>(),
    enabled: json['enabled'] as bool? ?? true,
  );

  Map<String, dynamic> toJson() => {
    'camera_id': cameraId,
    'mode': mode,
    'start_time': startTime,
    'end_time': endTime,
    'days_of_week': daysOfWeek,
    'enabled': enabled,
  };
}
```

- [ ] **Step 3: Create settings provider**

```dart
// clients/flutter/lib/providers/settings_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/user.dart';
import 'auth_provider.dart';

class SystemInfo {
  final String version;
  final String platform;
  final String uptime;
  final bool clipSearchAvailable;

  SystemInfo({this.version = '', this.platform = '', this.uptime = '', this.clipSearchAvailable = false});

  factory SystemInfo.fromJson(Map<String, dynamic> json) => SystemInfo(
    version: json['version'] as String? ?? '',
    platform: json['platform'] as String? ?? '',
    uptime: json['uptime'] as String? ?? '',
    clipSearchAvailable: json['clip_search_available'] as bool? ?? false,
  );
}

class StorageInfo {
  final int totalBytes;
  final int usedBytes;
  final int freeBytes;
  final int recordingsBytes;
  final bool warning;
  final bool critical;
  final List<CameraStorage> perCamera;

  StorageInfo({
    this.totalBytes = 0, this.usedBytes = 0, this.freeBytes = 0,
    this.recordingsBytes = 0, this.warning = false, this.critical = false,
    this.perCamera = const [],
  });

  factory StorageInfo.fromJson(Map<String, dynamic> json) => StorageInfo(
    totalBytes: json['total_bytes'] as int? ?? 0,
    usedBytes: json['used_bytes'] as int? ?? 0,
    freeBytes: json['free_bytes'] as int? ?? 0,
    recordingsBytes: json['recordings_bytes'] as int? ?? 0,
    warning: json['warning'] as bool? ?? false,
    critical: json['critical'] as bool? ?? false,
    perCamera: (json['per_camera'] as List?)?.map((e) => CameraStorage.fromJson(e as Map<String, dynamic>)).toList() ?? [],
  );

  double get usagePercent => totalBytes > 0 ? usedBytes / totalBytes : 0;
}

class CameraStorage {
  final String cameraId;
  final String cameraName;
  final int totalBytes;
  final int segmentCount;

  CameraStorage({this.cameraId = '', this.cameraName = '', this.totalBytes = 0, this.segmentCount = 0});

  factory CameraStorage.fromJson(Map<String, dynamic> json) => CameraStorage(
    cameraId: json['camera_id'] as String? ?? '',
    cameraName: json['camera_name'] as String? ?? '',
    totalBytes: json['total_bytes'] as int? ?? 0,
    segmentCount: json['segment_count'] as int? ?? 0,
  );
}

class AuditEntry {
  final int id;
  final String username;
  final String action;
  final String resourceType;
  final String resourceId;
  final String details;
  final String ipAddress;
  final String createdAt;

  AuditEntry({
    required this.id, this.username = '', this.action = '',
    this.resourceType = '', this.resourceId = '', this.details = '',
    this.ipAddress = '', this.createdAt = '',
  });

  factory AuditEntry.fromJson(Map<String, dynamic> json) => AuditEntry(
    id: json['id'] as int? ?? 0,
    username: json['username'] as String? ?? '',
    action: json['action'] as String? ?? '',
    resourceType: json['resource_type'] as String? ?? '',
    resourceId: json['resource_id'] as String? ?? '',
    details: json['details'] as String? ?? '',
    ipAddress: json['ip_address'] as String? ?? '',
    createdAt: json['created_at'] as String? ?? '',
  );
}

final systemInfoProvider = FutureProvider<SystemInfo>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return SystemInfo();
  final res = await api.get('/system/info');
  return SystemInfo.fromJson(res.data as Map<String, dynamic>);
});

final storageInfoProvider = FutureProvider<StorageInfo>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return StorageInfo();
  final res = await api.get('/system/storage');
  return StorageInfo.fromJson(res.data as Map<String, dynamic>);
});

final usersProvider = FutureProvider<List<User>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/users');
  return (res.data as List).map((e) => User.fromJson(e as Map<String, dynamic>)).toList();
});

final auditProvider = FutureProvider<List<AuditEntry>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/audit', queryParameters: {'limit': '100'});
    return (res.data as List).map((e) => AuditEntry.fromJson(e as Map<String, dynamic>)).toList();
  } catch (_) {
    return [];
  }
});
```

- [ ] **Step 4: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/models/zone.dart clients/flutter/lib/models/recording_rule.dart clients/flutter/lib/providers/settings_provider.dart
git commit -m "feat(flutter): add zone, recording rule models and settings providers"
```

---

### Task 2: Camera List Screen + Add Camera

**Files:**
- Create: `clients/flutter/lib/screens/cameras/camera_list_screen.dart`
- Create: `clients/flutter/lib/screens/cameras/add_camera_screen.dart`
- Create: `clients/flutter/lib/widgets/camera_status_badge.dart`
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Create camera status badge**

```dart
// clients/flutter/lib/widgets/camera_status_badge.dart
import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

class CameraStatusBadge extends StatelessWidget {
  final String status;
  const CameraStatusBadge({super.key, required this.status});

  @override
  Widget build(BuildContext context) {
    final online = status == 'online';
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 8, height: 8,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: online ? NvrColors.success : NvrColors.danger,
          ),
        ),
        const SizedBox(width: 4),
        Text(
          online ? 'Online' : 'Offline',
          style: TextStyle(
            color: online ? NvrColors.success : NvrColors.danger,
            fontSize: 11, fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Create camera list screen**

Camera list showing all cameras with status, AI badge, recording indicator. Tap → detail. FAB → add camera. Swipe → delete with confirmation.

```dart
// clients/flutter/lib/screens/cameras/camera_list_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../models/camera.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/camera_status_badge.dart';
import '../../widgets/notification_bell.dart';
import 'add_camera_screen.dart';
import 'camera_detail_screen.dart';

class CameraListScreen extends ConsumerWidget {
  const CameraListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final camerasAsync = ref.watch(camerasProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Cameras'),
        actions: const [NotificationBell()],
      ),
      floatingActionButton: FloatingActionButton(
        onPressed: () => Navigator.push(context, MaterialPageRoute(builder: (_) => const AddCameraScreen())),
        child: const Icon(Icons.add),
      ),
      body: camerasAsync.when(
        data: (cameras) {
          if (cameras.isEmpty) {
            return Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.camera_alt, size: 48, color: NvrColors.textMuted),
                  const SizedBox(height: 16),
                  const Text('No cameras yet'),
                  const SizedBox(height: 8),
                  ElevatedButton.icon(
                    onPressed: () => Navigator.push(context, MaterialPageRoute(builder: (_) => const AddCameraScreen())),
                    icon: const Icon(Icons.add),
                    label: const Text('Add Camera'),
                  ),
                ],
              ),
            );
          }
          return RefreshIndicator(
            onRefresh: () async => ref.invalidate(camerasProvider),
            child: ListView.builder(
              padding: const EdgeInsets.all(8),
              itemCount: cameras.length,
              itemBuilder: (_, i) => _CameraCard(camera: cameras[i], ref: ref),
            ),
          );
        },
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Error: $e')),
      ),
    );
  }
}

class _CameraCard extends StatelessWidget {
  final Camera camera;
  final WidgetRef ref;

  const _CameraCard({required this.camera, required this.ref});

  @override
  Widget build(BuildContext context) {
    return Dismissible(
      key: Key(camera.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 16),
        color: NvrColors.danger,
        child: const Icon(Icons.delete, color: Colors.white),
      ),
      confirmDismiss: (_) => showDialog<bool>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('Delete Camera'),
          content: Text('Delete "${camera.name}"? This cannot be undone.'),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
            TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('Delete', style: TextStyle(color: NvrColors.danger))),
          ],
        ),
      ),
      onDismissed: (_) async {
        final api = ref.read(apiClientProvider);
        if (api != null) {
          await api.delete('/cameras/${camera.id}');
          ref.invalidate(camerasProvider);
        }
      },
      child: Card(
        child: ListTile(
          leading: Icon(
            camera.status == 'online' ? Icons.videocam : Icons.videocam_off,
            color: camera.status == 'online' ? NvrColors.accent : NvrColors.textMuted,
          ),
          title: Text(camera.name),
          subtitle: CameraStatusBadge(status: camera.status),
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (camera.aiEnabled) const Icon(Icons.psychology, size: 16, color: NvrColors.accent),
              const SizedBox(width: 4),
              const Icon(Icons.chevron_right, color: NvrColors.textMuted),
            ],
          ),
          onTap: () => Navigator.push(
            context,
            MaterialPageRoute(builder: (_) => CameraDetailScreen(cameraId: camera.id)),
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 3: Create add camera screen**

Two tabs: ONVIF discovery + manual add. Discovery has 30s timeout + cancel.

```dart
// clients/flutter/lib/screens/cameras/add_camera_screen.dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';

class AddCameraScreen extends ConsumerStatefulWidget {
  const AddCameraScreen({super.key});
  @override
  ConsumerState<AddCameraScreen> createState() => _AddCameraScreenState();
}

class _AddCameraScreenState extends ConsumerState<AddCameraScreen> with SingleTickerProviderStateMixin {
  late TabController _tabController;

  // Discovery state
  bool _discovering = false;
  List<Map<String, dynamic>> _discovered = [];
  Timer? _timeout;

  // Manual add state
  final _nameController = TextEditingController();
  final _rtspController = TextEditingController();
  final _usernameController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _adding = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
  }

  @override
  void dispose() {
    _tabController.dispose();
    _timeout?.cancel();
    super.dispose();
  }

  Future<void> _discover() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() { _discovering = true; _discovered = []; });
    _timeout = Timer(const Duration(seconds: 30), () {
      if (mounted) setState(() => _discovering = false);
    });
    try {
      final res = await api.post('/cameras/discover');
      if (res.data is Map) {
        // Poll for results
        await Future.delayed(const Duration(seconds: 3));
        final results = await api.get('/cameras/discover/results');
        if (results.data is List) {
          setState(() => _discovered = (results.data as List).cast<Map<String, dynamic>>());
        }
      }
    } catch (_) {}
    _timeout?.cancel();
    setState(() => _discovering = false);
  }

  Future<void> _addManual() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    final rtsp = _rtspController.text.trim();
    if (!rtsp.startsWith('rtsp://')) {
      setState(() => _error = 'URL must start with rtsp://');
      return;
    }
    setState(() { _adding = true; _error = null; });
    try {
      await api.post('/cameras', data: {
        'name': _nameController.text.trim().isEmpty ? 'New Camera' : _nameController.text.trim(),
        'rtsp_url': rtsp,
        'onvif_username': _usernameController.text.trim(),
        'onvif_password': _passwordController.text,
      });
      ref.invalidate(camerasProvider);
      if (mounted) Navigator.pop(context);
    } catch (e) {
      setState(() => _error = 'Failed to add camera');
    }
    setState(() => _adding = false);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Add Camera'),
        bottom: TabBar(
          controller: _tabController,
          tabs: const [Tab(text: 'Discover'), Tab(text: 'Manual')],
        ),
      ),
      body: TabBarView(
        controller: _tabController,
        children: [
          // Discovery tab
          Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              children: [
                ElevatedButton(
                  onPressed: _discovering ? null : _discover,
                  child: _discovering
                      ? const Row(mainAxisSize: MainAxisSize.min, children: [
                          SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
                          SizedBox(width: 8),
                          Text('Discovering...'),
                        ])
                      : const Text('Start Discovery'),
                ),
                if (_discovering)
                  TextButton(onPressed: () { _timeout?.cancel(); setState(() => _discovering = false); }, child: const Text('Cancel')),
                const SizedBox(height: 16),
                Expanded(
                  child: ListView.builder(
                    itemCount: _discovered.length,
                    itemBuilder: (_, i) {
                      final d = _discovered[i];
                      return Card(
                        child: ListTile(
                          title: Text(d['name'] as String? ?? 'Unknown'),
                          subtitle: Text(d['xaddr'] as String? ?? ''),
                          trailing: const Icon(Icons.add_circle_outline, color: NvrColors.accent),
                          onTap: () {
                            // TODO: probe + add discovered camera
                          },
                        ),
                      );
                    },
                  ),
                ),
              ],
            ),
          ),
          // Manual tab
          SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextField(controller: _nameController, decoration: const InputDecoration(labelText: 'Camera Name', hintText: 'Front Door')),
                const SizedBox(height: 12),
                TextField(controller: _rtspController, decoration: const InputDecoration(labelText: 'RTSP URL', hintText: 'rtsp://192.168.1.100/stream')),
                const SizedBox(height: 12),
                TextField(controller: _usernameController, decoration: const InputDecoration(labelText: 'Username (optional)')),
                const SizedBox(height: 12),
                TextField(controller: _passwordController, decoration: const InputDecoration(labelText: 'Password (optional)'), obscureText: true),
                if (_error != null) Padding(padding: const EdgeInsets.only(top: 8), child: Text(_error!, style: const TextStyle(color: NvrColors.danger, fontSize: 13))),
                const SizedBox(height: 24),
                ElevatedButton(
                  onPressed: _adding ? null : _addManual,
                  child: _adding ? const CircularProgressIndicator(strokeWidth: 2) : const Text('Add Camera'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 4: Update router**

Replace `/cameras` route with `CameraListScreen`:
```dart
GoRoute(path: '/cameras', builder: (_, __) => const CameraListScreen()),
```
Import: `import '../screens/cameras/camera_list_screen.dart';`

- [ ] **Step 5: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/cameras/ clients/flutter/lib/widgets/camera_status_badge.dart clients/flutter/lib/router/
git commit -m "feat(flutter): add camera list with discovery and manual add"
```

---

### Task 3: Camera Detail + Recording Rules

**Files:**
- Create: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`
- Create: `clients/flutter/lib/screens/cameras/recording_rules_screen.dart`

- [ ] **Step 1: Create camera detail screen**

Tabbed view: General, Recording, AI, Zones, Advanced.

The implementer should:
- Read the camera by ID from the API on init
- Show tabs using `DefaultTabController` + `TabBarView`
- **General tab:** Edit name, RTSP URL, ONVIF endpoint (save button)
- **Recording tab:** Navigate to `RecordingRulesScreen`
- **AI tab:** Toggle AI enabled, sub-stream URL input, confidence slider (20-90%), save button
- **Zones tab:** Navigate to `ZoneEditorScreen` (Task 4)
- **Advanced tab:** Motion timeout slider, retention days input, save button

Each tab uses `apiFetch` via the `apiClientProvider` for saves (`PUT /cameras/:id`, `PUT /cameras/:id/ai`, etc).

- [ ] **Step 2: Create recording rules screen**

List of recording rules for a camera with add/edit/delete:
- Fetch via `GET /cameras/:id/recording-rules`
- Each rule shows mode (continuous/motion/schedule) + enabled toggle
- Add dialog: mode dropdown, start/end time pickers (for schedule), days of week chips
- Save via `POST /cameras/:id/recording-rules`
- Delete via `DELETE /recording-rules/:id`

- [ ] **Step 3: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/cameras/
git commit -m "feat(flutter): add camera detail with tabbed config and recording rules"
```

---

### Task 4: Zone Editor

**Files:**
- Create: `clients/flutter/lib/screens/cameras/zone_editor_screen.dart`

- [ ] **Step 1: Create zone editor with polygon drawing**

The zone editor screen:
1. Fetches camera snapshot from `GET /cameras/:id/snapshot` and displays as background image
2. Fetches existing zones from `GET /cameras/:id/zones`
3. Draws existing zones as semi-transparent colored polygon overlays using `CustomPainter`
4. New zone creation: tap to add points (small circles), tap near first point or double-tap to close polygon
5. After closing, prompt for zone name, then `POST /cameras/:id/zones`
6. Zone list panel (bottom sheet or side panel) showing zone names with delete buttons
7. Tap a zone → expand config: class toggles (person/car/dog), cooldown slider (0-300s), loiter slider (0-300s), enter/leave/loiter checkboxes
8. Save zone config via `PUT /zones/:id`

Key implementation notes:
- Use `GestureDetector` wrapping `CustomPaint` for the drawing canvas
- Store drawing points as `List<Offset>` in normalized 0-1 coords (divide by image size)
- Colors: cycle through `[blue, green, amber, red, purple, pink]` for different zones
- `CustomPainter` draws: snapshot image (if loaded), zone polygons, current drawing points+lines

```dart
// clients/flutter/lib/screens/cameras/zone_editor_screen.dart
// The implementer should create a ConsumerStatefulWidget with:
// - Image.network for snapshot (with error fallback)
// - CustomPaint + GestureDetector layered over the image
// - _ZonePainter draws: existing zones as filled polygons with alpha,
//   current drawing points as circles connected by lines
// - State tracks: snapshot loaded, existing zones, current drawing points,
//   selected zone for editing, zone config form values
// - API calls: GET zones, POST zone, PUT zone, DELETE zone
```

- [ ] **Step 2: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/cameras/zone_editor_screen.dart
git commit -m "feat(flutter): add zone editor with polygon drawing on camera snapshot"
```

---

### Task 5: Settings Screen (System, Storage, Backups, Audit)

**Files:**
- Create: `clients/flutter/lib/screens/settings/settings_screen.dart`
- Create: `clients/flutter/lib/screens/settings/storage_panel.dart`
- Create: `clients/flutter/lib/screens/settings/backup_panel.dart`
- Create: `clients/flutter/lib/screens/settings/audit_panel.dart`
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Create settings screen with tabs**

```dart
// clients/flutter/lib/screens/settings/settings_screen.dart
// DefaultTabController with 5 tabs: System, Storage, Users, Backups, Audit
// System tab: shows version, platform, uptime, server URL from systemInfoProvider
// Other tabs delegate to their panel widgets
```

- [ ] **Step 2: Create storage panel**

Shows disk usage progress bar (colored green/yellow/red by threshold), per-camera storage breakdown as a list, recordings size. Uses `storageInfoProvider`.

- [ ] **Step 3: Create backup panel**

- "Create Backup" button → `POST /system/backup`
- List of backups from `GET /system/backups`
- Download link for each → opens URL in browser or triggers download

- [ ] **Step 4: Create audit panel**

- DataTable/ListView of audit entries from `auditProvider`
- Columns: time, user, action, resource, IP
- "Export CSV" button that generates CSV string and triggers download/share

```dart
void _exportCsv(List<AuditEntry> entries) {
  final headers = 'Timestamp,User,Action,Resource,Details,IP\n';
  final rows = entries.map((e) =>
    '"${e.createdAt}","${e.username}","${e.action}","${e.resourceType}/${e.resourceId}","${e.details}","${e.ipAddress}"'
  ).join('\n');
  final csv = headers + rows;
  // Use share_plus or file_saver to export
  // For simplicity, copy to clipboard with a snackbar confirmation
}
```

- [ ] **Step 5: Update router + verify + commit**

Replace `/settings` route with `SettingsScreen`.

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/settings/ clients/flutter/lib/router/
git commit -m "feat(flutter): add settings with system info, storage, backups, and audit log"
```

---

### Task 6: User Management

**Files:**
- Create: `clients/flutter/lib/screens/settings/user_management_screen.dart`

- [ ] **Step 1: Create user management screen**

Admin-only screen (check `user.role == 'admin'` from auth provider):
- List of users from `usersProvider`
- Each shows: avatar initials, username, role badge
- Create user dialog: username, password, role dropdown (admin/viewer), camera permissions
- Edit user: change role, permissions
- Delete user: confirmation dialog → `DELETE /users/:id`
- Change own password section at top: current password + new password + confirm → `PUT /auth/password`

The implementer should use `showDialog` or `showModalBottomSheet` for create/edit forms.

- [ ] **Step 2: Wire into settings screen**

The "Users" tab in `settings_screen.dart` should render `UserManagementScreen()` as a widget (not a separate page, since it's a tab).

- [ ] **Step 3: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/settings/user_management_screen.dart clients/flutter/lib/screens/settings/settings_screen.dart
git commit -m "feat(flutter): add user management with CRUD and password change"
```

---

### Task 7: Final Verification

- [ ] **Step 1: Full analyze**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 2: Verify all routes work**

Check `app_router.dart` has all 5 tabs pointing to real screens:
- `/live` → `LiveViewScreen`
- `/playback` → `PlaybackScreen`
- `/search` → `ClipSearchScreen`
- `/cameras` → `CameraListScreen`
- `/settings` → `SettingsScreen`

- [ ] **Step 3: File count check**

```bash
find clients/flutter/lib -name "*.dart" ! -name "*.freezed.dart" ! -name "*.g.dart" | wc -l
```

Expected: ~55-60 source files.

- [ ] **Step 4: Commit any final fixes**

```bash
git add -A && git commit -m "feat(flutter): finalize management + settings, complete Flutter client"
```
