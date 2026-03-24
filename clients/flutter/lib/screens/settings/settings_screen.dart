import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/notification_bell.dart';
import 'audit_panel.dart';
import 'backup_panel.dart';
import 'storage_panel.dart';
import 'user_management_screen.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return DefaultTabController(
      length: 5,
      child: Scaffold(
        backgroundColor: NvrColors.bgPrimary,
        appBar: AppBar(
          backgroundColor: NvrColors.bgSecondary,
          elevation: 0,
          title: const Text(
            'Settings',
            style: TextStyle(
              color: NvrColors.textPrimary,
              fontSize: 18,
              fontWeight: FontWeight.w600,
            ),
          ),
          actions: const [
            NotificationBell(),
            SizedBox(width: 8),
          ],
          bottom: const TabBar(
            isScrollable: true,
            tabAlignment: TabAlignment.start,
            indicatorColor: NvrColors.accent,
            labelColor: NvrColors.accent,
            unselectedLabelColor: NvrColors.textMuted,
            labelStyle: TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
            tabs: [
              Tab(text: 'System'),
              Tab(text: 'Storage'),
              Tab(text: 'Users'),
              Tab(text: 'Backups'),
              Tab(text: 'Audit'),
            ],
          ),
        ),
        body: const TabBarView(
          children: [
            _SystemTab(),
            StoragePanel(),
            UserManagementScreen(),
            BackupPanel(),
            AuditPanel(),
          ],
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────
// System Tab
// ─────────────────────────────────────────────

class _SystemTab extends ConsumerWidget {
  const _SystemTab();

  String _formatUptime(int seconds) {
    if (seconds <= 0) return '—';
    final d = seconds ~/ 86400;
    final h = (seconds % 86400) ~/ 3600;
    final m = (seconds % 3600) ~/ 60;
    final s = seconds % 60;
    if (d > 0) return '${d}d ${h}h ${m}m';
    if (h > 0) return '${h}h ${m}m ${s}s';
    if (m > 0) return '${m}m ${s}s';
    return '${s}s';
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final systemAsync = ref.watch(systemInfoProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '—';

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Server URL card
        _InfoCard(
          title: 'Connection',
          icon: Icons.dns_outlined,
          children: [
            _InfoRow(label: 'Server URL', value: serverUrl),
          ],
        ),
        const SizedBox(height: 12),
        // System info card
        systemAsync.when(
          loading: () => const Center(
            child: Padding(
              padding: EdgeInsets.all(32),
              child: CircularProgressIndicator(),
            ),
          ),
          error: (e, _) => Card(
            color: NvrColors.bgSecondary,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
              side: const BorderSide(color: NvrColors.border),
            ),
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  const Icon(Icons.error_outline, color: NvrColors.danger, size: 20),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Failed to load system info: $e',
                      style: const TextStyle(color: NvrColors.danger, fontSize: 13),
                    ),
                  ),
                ],
              ),
            ),
          ),
          data: (info) => _InfoCard(
            title: 'System Info',
            icon: Icons.info_outline,
            children: [
              _InfoRow(label: 'Version', value: info.version.isNotEmpty ? info.version : '—'),
              _InfoRow(label: 'Platform', value: info.platform.isNotEmpty ? info.platform : '—'),
              _InfoRow(label: 'Uptime', value: _formatUptime(info.uptime)),
              _InfoRow(
                label: 'Semantic Search',
                value: info.clipSearchAvailable ? 'Available' : 'Unavailable',
                valueColor: info.clipSearchAvailable ? NvrColors.success : NvrColors.textMuted,
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
        // Logged-in user card
        if (auth.user != null)
          _InfoCard(
            title: 'Current User',
            icon: Icons.account_circle_outlined,
            children: [
              _InfoRow(label: 'Username', value: auth.user!.username),
              _InfoRow(
                label: 'Role',
                value: auth.user!.role.toUpperCase(),
                valueColor: auth.user!.role == 'admin'
                    ? NvrColors.accent
                    : NvrColors.textSecondary,
              ),
            ],
          ),
      ],
    );
  }
}

class _InfoCard extends StatelessWidget {
  final String title;
  final IconData icon;
  final List<Widget> children;

  const _InfoCard({
    required this.title,
    required this.icon,
    required this.children,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      color: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: NvrColors.border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon, color: NvrColors.accent, size: 18),
                const SizedBox(width: 8),
                Text(
                  title,
                  style: const TextStyle(
                    color: NvrColors.textPrimary,
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            const Divider(color: NvrColors.border, height: 1),
            const SizedBox(height: 8),
            ...children,
          ],
        ),
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  final String label;
  final String value;
  final Color? valueColor;

  const _InfoRow({required this.label, required this.value, this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          SizedBox(
            width: 130,
            child: Text(
              label,
              style: const TextStyle(
                color: NvrColors.textMuted,
                fontSize: 13,
              ),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: TextStyle(
                color: valueColor ?? NvrColors.textPrimary,
                fontSize: 13,
                fontWeight: FontWeight.w500,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }
}
