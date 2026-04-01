import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_button.dart';

class DeviceMgmtSection extends ConsumerStatefulWidget {
  const DeviceMgmtSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  ConsumerState<DeviceMgmtSection> createState() => _DeviceMgmtSectionState();
}

class _DeviceMgmtSectionState extends ConsumerState<DeviceMgmtSection> {
  late final TextEditingController _hostnameCtrl;
  bool _savingHostname = false;
  bool _rebooting = false;

  @override
  void initState() {
    super.initState();
    _hostnameCtrl = TextEditingController();
  }

  @override
  void dispose() {
    _hostnameCtrl.dispose();
    super.dispose();
  }

  // ── System ─────────────────────────────────────────────────────────────────

  Future<void> _saveHostname() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _savingHostname = true);
    try {
      await api.put(
        '/cameras/${widget.cameraId}/device/hostname',
        data: {'name': _hostnameCtrl.text.trim()},
      );
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            backgroundColor: NvrColors.success,
            content: Text('Hostname saved'),
          ),
        );
        ref.invalidate(deviceHostnameProvider(widget.cameraId));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Failed to save hostname: $e'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _savingHostname = false);
    }
  }

  Future<void> _confirmReboot() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('REBOOT DEVICE', style: NvrTypography.monoSection),
        content: const Text(
          'This will reboot the camera. The stream will be interrupted.\nAre you sure?',
          style: NvrTypography.body,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('CANCEL',
                style: TextStyle(color: NvrColors.textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('REBOOT',
                style: TextStyle(color: NvrColors.danger)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _rebooting = true);
    try {
      await api.post(
        '/cameras/${widget.cameraId}/device/reboot',
        data: {'confirm': true},
      );
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            backgroundColor: NvrColors.warning,
            content: Text('Reboot command sent'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Reboot failed: $e'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _rebooting = false);
    }
  }

  // ── Users ──────────────────────────────────────────────────────────────────

  Future<void> _showAddUserDialog() async {
    final usernameCtrl = TextEditingController();
    final passwordCtrl = TextEditingController();
    String selectedRole = 'Operator';

    await showDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setStateDialog) => AlertDialog(
          backgroundColor: NvrColors.bgSecondary,
          title: const Text('ADD USER', style: NvrTypography.monoSection),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              _DialogField(controller: usernameCtrl, label: 'USERNAME'),
              const SizedBox(height: 10),
              _DialogField(
                  controller: passwordCtrl,
                  label: 'PASSWORD',
                  obscure: true),
              const SizedBox(height: 10),
              DropdownButtonFormField<String>(
                value: selectedRole,
                dropdownColor: NvrColors.bgTertiary,
                style: NvrTypography.monoData,
                decoration: InputDecoration(
                  labelText: 'ROLE',
                  labelStyle: NvrTypography.monoLabel,
                  enabledBorder: OutlineInputBorder(
                    borderSide:
                        const BorderSide(color: NvrColors.border),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  focusedBorder: OutlineInputBorder(
                    borderSide:
                        const BorderSide(color: NvrColors.accent),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  filled: true,
                  fillColor: NvrColors.bgInput,
                ),
                items: const ['Administrator', 'Operator', 'User']
                    .map((r) => DropdownMenuItem(value: r, child: Text(r)))
                    .toList(),
                onChanged: (v) {
                  if (v != null) {
                    setStateDialog(() => selectedRole = v);
                  }
                },
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('CANCEL',
                  style: TextStyle(color: NvrColors.textSecondary)),
            ),
            TextButton(
              onPressed: () async {
                final api = ref.read(apiClientProvider);
                if (api == null) return;
                final nav = Navigator.of(ctx);
                final messenger = ScaffoldMessenger.of(context);
                try {
                  await api.post(
                    '/cameras/${widget.cameraId}/device/users',
                    data: {
                      'username': usernameCtrl.text.trim(),
                      'password': passwordCtrl.text,
                      'role': selectedRole,
                    },
                  );
                  if (mounted) {
                    ref.invalidate(deviceUsersProvider(widget.cameraId));
                    nav.pop();
                    messenger.showSnackBar(
                      const SnackBar(
                        backgroundColor: NvrColors.success,
                        content: Text('User added'),
                      ),
                    );
                  }
                } catch (e) {
                  if (mounted) {
                    messenger.showSnackBar(
                      SnackBar(
                        backgroundColor: NvrColors.danger,
                        content: Text('Failed to add user: $e'),
                      ),
                    );
                  }
                }
              },
              child: const Text('ADD',
                  style: TextStyle(color: NvrColors.accent)),
            ),
          ],
        ),
      ),
    );

    usernameCtrl.dispose();
    passwordCtrl.dispose();
  }

  Future<void> _confirmDeleteUser(String username) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('DELETE USER', style: NvrTypography.monoSection),
        content: Text(
          'Delete user "$username"?',
          style: NvrTypography.body,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('CANCEL',
                style: TextStyle(color: NvrColors.textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('DELETE',
                style: TextStyle(color: NvrColors.danger)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/cameras/${widget.cameraId}/device/users/$username');
      if (mounted) {
        ref.invalidate(deviceUsersProvider(widget.cameraId));
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.success,
            content: Text('User "$username" deleted'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Failed to delete user: $e'),
          ),
        );
      }
    }
  }

  // ── Build ──────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildSystemSection(),
        const SizedBox(height: 8),
        _buildNetworkSection(),
        const SizedBox(height: 8),
        _buildUsersSection(),
      ],
    );
  }

  Widget _buildSystemSection() {
    final dateTimeAsync = ref.watch(deviceDateTimeProvider(widget.cameraId));
    final hostnameAsync = ref.watch(deviceHostnameProvider(widget.cameraId));

    // Pre-fill hostname field when data arrives
    hostnameAsync.whenData((info) {
      if (info != null && _hostnameCtrl.text.isEmpty) {
        _hostnameCtrl.text = info.name;
      }
    });

    return _SubSection(
      title: 'SYSTEM',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Date/time
          dateTimeAsync.when(
            loading: () =>
                const Text('Loading...', style: NvrTypography.body),
            error: (_, __) => const SizedBox.shrink(),
            data: (dt) {
              if (dt == null) return const SizedBox.shrink();
              return Column(
                children: [
                  _InfoRow(label: 'TIME TYPE', value: dt.type),
                  const SizedBox(height: 6),
                  _InfoRow(label: 'TIMEZONE', value: dt.timezone),
                  const SizedBox(height: 6),
                  _InfoRow(label: 'UTC TIME', value: dt.utcTime),
                  const SizedBox(height: 12),
                ],
              );
            },
          ),
          // Hostname
          Text('HOSTNAME', style: NvrTypography.monoLabel),
          const SizedBox(height: 6),
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _hostnameCtrl,
                  style: NvrTypography.monoData,
                  decoration: InputDecoration(
                    filled: true,
                    fillColor: NvrColors.bgInput,
                    enabledBorder: OutlineInputBorder(
                      borderSide:
                          const BorderSide(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderSide:
                          const BorderSide(color: NvrColors.accent),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 10, vertical: 8),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              HudButton(
                label: _savingHostname ? 'SAVING...' : 'SAVE',
                style: HudButtonStyle.secondary,
                onPressed: _savingHostname ? null : _saveHostname,
              ),
            ],
          ),
          const SizedBox(height: 16),
          // Reboot
          HudButton(
            label: _rebooting ? 'REBOOTING...' : 'REBOOT DEVICE',
            style: HudButtonStyle.danger,
            icon: Icons.restart_alt,
            onPressed: _rebooting ? null : _confirmReboot,
          ),
        ],
      ),
    );
  }

  Widget _buildNetworkSection() {
    final interfacesAsync =
        ref.watch(networkInterfacesProvider(widget.cameraId));
    final protocolsAsync =
        ref.watch(networkProtocolsProvider(widget.cameraId));

    return _SubSection(
      title: 'NETWORK',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Interfaces
          Text('INTERFACES', style: NvrTypography.monoLabel),
          const SizedBox(height: 8),
          interfacesAsync.when(
            loading: () =>
                const Text('Loading...', style: NvrTypography.body),
            error: (_, __) => const SizedBox.shrink(),
            data: (interfaces) {
              if (interfaces.isEmpty) {
                return const Text('No interfaces', style: NvrTypography.body);
              }
              return Column(
                children: interfaces.map((iface) {
                  final ipAddr = iface.ipv4 != null
                      ? '${iface.ipv4!.address}/${iface.ipv4!.prefix}'
                      : '—';
                  final dhcpLabel =
                      (iface.ipv4?.dhcp ?? false) ? 'DHCP' : 'Static';
                  return Container(
                    margin: const EdgeInsets.only(bottom: 6),
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(
                      color: NvrColors.bgTertiary,
                      borderRadius: BorderRadius.circular(4),
                      border: Border.all(color: NvrColors.border),
                    ),
                    child: Column(
                      children: [
                        _InfoRow(label: 'TOKEN', value: iface.token),
                        const SizedBox(height: 4),
                        _InfoRow(label: 'MAC', value: iface.mac),
                        const SizedBox(height: 4),
                        _InfoRow(label: 'IP', value: ipAddr),
                        const SizedBox(height: 4),
                        _InfoRow(label: 'MODE', value: dhcpLabel),
                      ],
                    ),
                  );
                }).toList(),
              );
            },
          ),
          const SizedBox(height: 12),
          // Protocols
          Text('PROTOCOLS', style: NvrTypography.monoLabel),
          const SizedBox(height: 8),
          protocolsAsync.when(
            loading: () =>
                const Text('Loading...', style: NvrTypography.body),
            error: (_, __) => const SizedBox.shrink(),
            data: (protocols) {
              if (protocols.isEmpty) {
                return const Text('No protocols', style: NvrTypography.body);
              }
              return Column(
                children: protocols.map((proto) {
                  return Container(
                    margin: const EdgeInsets.only(bottom: 6),
                    padding: const EdgeInsets.symmetric(
                        horizontal: 10, vertical: 8),
                    decoration: BoxDecoration(
                      color: NvrColors.bgTertiary,
                      borderRadius: BorderRadius.circular(4),
                      border: Border.all(color: NvrColors.border),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(proto.name,
                              style: NvrTypography.monoData),
                        ),
                        Text(
                          proto.port > 0 ? ':${proto.port}' : '',
                          style: NvrTypography.monoLabel,
                        ),
                        const SizedBox(width: 8),
                        _StatusDot(enabled: proto.enabled),
                      ],
                    ),
                  );
                }).toList(),
              );
            },
          ),
        ],
      ),
    );
  }

  Widget _buildUsersSection() {
    final usersAsync = ref.watch(deviceUsersProvider(widget.cameraId));

    return _SubSection(
      title: 'DEVICE USERS',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          usersAsync.when(
            loading: () =>
                const Text('Loading...', style: NvrTypography.body),
            error: (_, __) => const SizedBox.shrink(),
            data: (users) {
              if (users.isEmpty) {
                return const Padding(
                  padding: EdgeInsets.only(bottom: 12),
                  child: Text('No users found', style: NvrTypography.body),
                );
              }
              return Column(
                children: [
                  ...users.map((user) => Container(
                        margin: const EdgeInsets.only(bottom: 6),
                        padding: const EdgeInsets.symmetric(
                            horizontal: 10, vertical: 8),
                        decoration: BoxDecoration(
                          color: NvrColors.bgTertiary,
                          borderRadius: BorderRadius.circular(4),
                          border: Border.all(color: NvrColors.border),
                        ),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(user.username,
                                  style: NvrTypography.monoData),
                            ),
                            _RoleBadge(role: user.role),
                            const SizedBox(width: 8),
                            InkWell(
                              onTap: () =>
                                  _confirmDeleteUser(user.username),
                              borderRadius: BorderRadius.circular(4),
                              child: const Padding(
                                padding: EdgeInsets.all(4),
                                child: Icon(
                                  Icons.delete_outline,
                                  size: 16,
                                  color: NvrColors.danger,
                                ),
                              ),
                            ),
                          ],
                        ),
                      )),
                  const SizedBox(height: 4),
                ],
              );
            },
          ),
          HudButton(
            label: 'ADD USER',
            style: HudButtonStyle.secondary,
            icon: Icons.person_add_outlined,
            onPressed: _showAddUserDialog,
          ),
        ],
      ),
    );
  }
}

// ── Supporting widgets ────────────────────────────────────────────────────────

class _SubSection extends StatelessWidget {
  const _SubSection({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgTertiary,
        border: Border.all(color: NvrColors.border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 10, 12, 8),
            child: Text(title, style: NvrTypography.monoSection),
          ),
          const Divider(height: 1, color: NvrColors.border),
          Padding(
            padding: const EdgeInsets.all(12),
            child: child,
          ),
        ],
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 100,
          child: Text(label, style: NvrTypography.monoLabel),
        ),
        Expanded(
          child: Text(
            value.isEmpty ? '—' : value,
            style: NvrTypography.monoData,
          ),
        ),
      ],
    );
  }
}

class _StatusDot extends StatelessWidget {
  const _StatusDot({required this.enabled});

  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 6,
      height: 6,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: enabled ? NvrColors.success : NvrColors.textMuted,
      ),
    );
  }
}

class _RoleBadge extends StatelessWidget {
  const _RoleBadge({required this.role});

  final String role;

  @override
  Widget build(BuildContext context) {
    final color = switch (role.toLowerCase()) {
      'administrator' => NvrColors.danger,
      'operator' => NvrColors.warning,
      _ => NvrColors.textSecondary,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.12),
        border: Border.all(color: color.withOpacity(0.35)),
        borderRadius: BorderRadius.circular(3),
      ),
      child: Text(
        role.toUpperCase(),
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w600,
          letterSpacing: 1,
          color: color,
        ),
      ),
    );
  }
}

class _DialogField extends StatelessWidget {
  const _DialogField({
    required this.controller,
    required this.label,
    this.obscure = false,
  });

  final TextEditingController controller;
  final String label;
  final bool obscure;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: obscure,
      style: NvrTypography.monoData,
      decoration: InputDecoration(
        labelText: label,
        labelStyle: NvrTypography.monoLabel,
        filled: true,
        fillColor: NvrColors.bgInput,
        enabledBorder: OutlineInputBorder(
          borderSide: const BorderSide(color: NvrColors.border),
          borderRadius: BorderRadius.circular(4),
        ),
        focusedBorder: OutlineInputBorder(
          borderSide: const BorderSide(color: NvrColors.accent),
          borderRadius: BorderRadius.circular(4),
        ),
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      ),
    );
  }
}
