import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/settings_provider.dart';
import '../../providers/theme_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/responsive.dart';
import 'audit_panel.dart';
import 'backup_panel.dart';
import 'performance_panel.dart';
import 'preferences_panel.dart';
import 'storage_panel.dart';
import 'user_management_screen.dart';

// ─────────────────────────────────────────────
// SettingsScreen — sidebar nav on desktop, pill tabs on mobile
// ─────────────────────────────────────────────

class SettingsScreen extends ConsumerStatefulWidget {
  const SettingsScreen({super.key});

  @override
  ConsumerState<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends ConsumerState<SettingsScreen> {
  int _selectedSection = 0;

  static const _sections = ['System', 'Preferences', 'Storage', 'Performance', 'Users', 'Backups', 'Audit Log'];

  Widget _buildContent() {
    switch (_selectedSection) {
      case 0:
        return const _SystemPanel();
      case 1:
        return const PreferencesPanel();
      case 2:
        return const StoragePanel();
      case 3:
        return const PerformancePanel();
      case 4:
        return const UserManagementScreen();
      case 5:
        return const BackupPanel();
      case 6:
        return const AuditPanel();
      default:
        return const SizedBox.shrink();
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = !Responsive.isPhone(context);

    return Scaffold(
      backgroundColor: NvrColors.of(context).bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.of(context).bgSecondary,
        elevation: 0,
        title: Text('SETTINGS', style: NvrTypography.of(context).pageTitle),
        actions: const [],
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(1),
          child: Container(height: 1, color: NvrColors.of(context).border),
        ),
      ),
      body: isDesktop ? _buildDesktopLayout() : _buildMobileLayout(),
    );
  }

  Widget _buildDesktopLayout() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Sidebar
        SizedBox(
          width: 180,
          child: Container(
            decoration: BoxDecoration(
              color: NvrColors.of(context).bgSecondary,
              border: Border(right: BorderSide(color: NvrColors.of(context).border)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const SizedBox(height: 16),
                ..._sections.asMap().entries.map((e) {
                  final idx = e.key;
                  final label = e.value;
                  final isActive = idx == _selectedSection;
                  return _SidebarItem(
                    label: label,
                    isActive: isActive,
                    onTap: () => setState(() => _selectedSection = idx),
                  );
                }),
              ],
            ),
          ),
        ),
        // Content area
        Expanded(child: _buildContent()),
      ],
    );
  }

  Widget _buildMobileLayout() {
    return Column(
      children: [
        // Horizontal pill tabs
        Container(
          color: NvrColors.of(context).bgSecondary,
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            child: Row(
              children: _sections.asMap().entries.map((e) {
                final idx = e.key;
                final label = e.value;
                final isActive = idx == _selectedSection;
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: GestureDetector(
                    onTap: () => setState(() => _selectedSection = idx),
                    child: Container(
                      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
                      decoration: BoxDecoration(
                        color: isActive
                            ? NvrColors.of(context).accent.withOpacity(0.12)
                            : Colors.transparent,
                        border: Border.all(
                          color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).border,
                        ),
                        borderRadius: BorderRadius.circular(20),
                      ),
                      child: Text(
                        label.toUpperCase(),
                        style: NvrTypography.of(context).monoLabel.copyWith(
                          color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textSecondary,
                          letterSpacing: 1.0,
                        ),
                      ),
                    ),
                  ),
                );
              }).toList(),
            ),
          ),
        ),
        Container(height: 1, color: NvrColors.of(context).border),
        // Content
        Expanded(child: _buildContent()),
      ],
    );
  }
}

// ─────────────────────────────────────────────
// Sidebar item
// ─────────────────────────────────────────────

class _SidebarItem extends StatelessWidget {
  final String label;
  final bool isActive;
  final VoidCallback onTap;

  const _SidebarItem({
    required this.label,
    required this.isActive,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        height: 44,
        decoration: BoxDecoration(
          color: isActive ? NvrColors.of(context).accent.withOpacity(0.07) : Colors.transparent,
          border: Border(
            right: BorderSide(
              color: isActive ? NvrColors.of(context).accent : Colors.transparent,
              width: 2,
            ),
          ),
        ),
        alignment: Alignment.centerLeft,
        padding: const EdgeInsets.symmetric(horizontal: 20),
        child: Text(
          label.toUpperCase(),
          style: NvrTypography.of(context).monoLabel.copyWith(
            color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textSecondary,
            letterSpacing: 1.2,
          ),
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────
// System panel
// ─────────────────────────────────────────────

class _SystemPanel extends ConsumerWidget {
  const _SystemPanel();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final systemAsync = ref.watch(systemInfoProvider);
    final auth = ref.watch(authProvider);
    final camerasAsync = ref.watch(camerasProvider);
    final serverUrl = auth.serverUrl ?? '—';

    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        // ── Stats grid ──
        systemAsync.when(
          loading: () => Center(
            child: Padding(
              padding: EdgeInsets.all(32),
              child: CircularProgressIndicator(color: NvrColors.of(context).accent),
            ),
          ),
          error: (e, _) => _HudCard(
            child: Row(
              children: [
                Icon(Icons.error_outline, color: NvrColors.of(context).danger, size: 16),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    'Failed to load system info: $e',
                    style: NvrTypography.of(context).body.copyWith(color: NvrColors.of(context).danger),
                  ),
                ),
              ],
            ),
          ),
          data: (info) {
            final onlineCount = camerasAsync.valueOrNull
                    ?.where((c) => c.status == 'connected')
                    .length ??
                0;
            final totalCount = camerasAsync.valueOrNull?.length ?? 0;
            final offlineCount = totalCount - onlineCount;

            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // 3-column stat grid
                Row(
                  children: [
                    Expanded(
                      child: _StatTile(
                        label: 'VERSION',
                        child: Text(
                          info.version.isNotEmpty ? info.version : '—',
                          style: NvrTypography.of(context).monoDataLarge,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: _StatTile(
                        label: 'UPTIME',
                        child: Text(
                          info.uptimeFormatted,
                          style: NvrTypography.of(context).monoDataLarge.copyWith(
                            color: NvrColors.of(context).success,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: _StatTile(
                        label: 'CAMERAS',
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Text(
                              '$onlineCount',
                              style: NvrTypography.of(context).monoDataLarge.copyWith(
                                color: NvrColors.of(context).success,
                              ),
                            ),
                            const SizedBox(width: 4),
                            Icon(
                              Icons.arrow_upward,
                              size: 12,
                              color: NvrColors.of(context).success,
                            ),
                            const SizedBox(width: 8),
                            Text(
                              '$offlineCount',
                              style: NvrTypography.of(context).monoDataLarge.copyWith(
                                color: offlineCount > 0
                                    ? NvrColors.of(context).danger
                                    : NvrColors.of(context).textMuted,
                              ),
                            ),
                            const SizedBox(width: 4),
                            Icon(
                              Icons.arrow_downward,
                              size: 12,
                              color: offlineCount > 0
                                  ? NvrColors.of(context).danger
                                  : NvrColors.of(context).textMuted,
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 20),
                // System details card
                _HudCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('SYSTEM INFO', style: NvrTypography.of(context).monoSection),
                      const SizedBox(height: 12),
                      Container(height: 1, color: NvrColors.of(context).border),
                      const SizedBox(height: 10),
                      _DataRow(label: 'PLATFORM', value: info.platform.isNotEmpty ? info.platform : '—'),
                      _DataRow(
                        label: 'CLIP SEARCH',
                        value: info.clipSearchAvailable ? 'AVAILABLE' : 'UNAVAILABLE',
                        valueColor: info.clipSearchAvailable
                            ? NvrColors.of(context).success
                            : NvrColors.of(context).textMuted,
                      ),
                    ],
                  ),
                ),
              ],
            );
          },
        ),
        const SizedBox(height: 20),
        // Connection card
        _HudCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('CONNECTION', style: NvrTypography.of(context).monoSection),
              const SizedBox(height: 12),
              Container(height: 1, color: NvrColors.of(context).border),
              const SizedBox(height: 10),
              _DataRow(label: 'SERVER URL', value: serverUrl),
            ],
          ),
        ),
        // Current user card
        if (auth.user != null) ...[
          const SizedBox(height: 20),
          _HudCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('CURRENT USER', style: NvrTypography.of(context).monoSection),
                const SizedBox(height: 12),
                Container(height: 1, color: NvrColors.of(context).border),
                const SizedBox(height: 10),
                _DataRow(label: 'USERNAME', value: auth.user!.username),
                _DataRow(
                  label: 'ROLE',
                  value: auth.user!.role.toUpperCase(),
                  valueColor: auth.user!.role == 'admin'
                      ? NvrColors.of(context).accent
                      : NvrColors.of(context).textSecondary,
                ),
              ],
            ),
          ),
        ],
        // Appearance card
        const SizedBox(height: 20),
        _AppearanceCard(),
      ],
    );
  }
}

// ─────────────────────────────────────────────
// Appearance card — theme toggle
// ─────────────────────────────────────────────

class _AppearanceCard extends ConsumerWidget {
  const _AppearanceCard();

  static const _modes = [
    (mode: ThemeMode.system, label: 'SYSTEM', icon: Icons.brightness_auto),
    (mode: ThemeMode.dark, label: 'DARK', icon: Icons.dark_mode),
    (mode: ThemeMode.light, label: 'LIGHT', icon: Icons.light_mode),
  ];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);
    final currentMode = ref.watch(themeModeProvider);

    return _HudCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('APPEARANCE', style: typo.monoSection),
          const SizedBox(height: 12),
          Container(height: 1, color: colors.border),
          const SizedBox(height: 14),
          Row(
            children: _modes.map((entry) {
              final isActive = currentMode == entry.mode;
              return Expanded(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  child: GestureDetector(
                    onTap: () => ref
                        .read(themeModeProvider.notifier)
                        .setThemeMode(entry.mode),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 200),
                      curve: Curves.easeOut,
                      padding: const EdgeInsets.symmetric(
                          vertical: 10, horizontal: 8),
                      decoration: BoxDecoration(
                        color: isActive
                            ? colors.accent.withOpacity(0.12)
                            : Colors.transparent,
                        border: Border.all(
                          color:
                              isActive ? colors.accent : colors.border,
                        ),
                        borderRadius: BorderRadius.circular(6),
                      ),
                      child: Column(
                        children: [
                          Icon(
                            entry.icon,
                            size: 20,
                            color: isActive
                                ? colors.accent
                                : colors.textSecondary,
                          ),
                          const SizedBox(height: 6),
                          Text(
                            entry.label,
                            style: typo.monoLabel.copyWith(
                              color: isActive
                                  ? colors.accent
                                  : colors.textSecondary,
                              letterSpacing: 1.0,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              );
            }).toList(),
          ),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────
// Shared HUD primitives (local to this file)
// ─────────────────────────────────────────────

class _HudCard extends StatelessWidget {
  final Widget child;

  const _HudCard({required this.child});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: child,
    );
  }
}

class _StatTile extends StatelessWidget {
  final String label;
  final Widget child;

  const _StatTile({required this.label, required this.child});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: NvrTypography.of(context).monoLabel),
          const SizedBox(height: 8),
          child,
        ],
      ),
    );
  }
}

class _DataRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _DataRow({
    required this.label,
    required this.value,
    this.valueColor,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          SizedBox(
            width: 130,
            child: Text(label, style: NvrTypography.of(context).monoLabel),
          ),
          Expanded(
            child: Text(
              value,
              style: NvrTypography.of(context).monoData.copyWith(
                color: valueColor ?? NvrColors.of(context).textPrimary,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }
}
