import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/user.dart';
import '../../providers/auth_provider.dart';
import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

class UserManagementScreen extends ConsumerStatefulWidget {
  const UserManagementScreen({super.key});

  @override
  ConsumerState<UserManagementScreen> createState() =>
      _UserManagementScreenState();
}

class _UserManagementScreenState extends ConsumerState<UserManagementScreen> {
  // Change password fields
  final _currentPasswordCtrl = TextEditingController();
  final _newPasswordCtrl = TextEditingController();
  final _confirmPasswordCtrl = TextEditingController();
  bool _changingPassword = false;
  String? _passwordError;
  bool _passwordSuccess = false;

  // Visibility toggles
  bool _showCurrentPw = false;
  bool _showNewPw = false;
  bool _showConfirmPw = false;

  @override
  void dispose() {
    _currentPasswordCtrl.dispose();
    _newPasswordCtrl.dispose();
    _confirmPasswordCtrl.dispose();
    super.dispose();
  }

  Future<void> _changePassword() async {
    final current = _currentPasswordCtrl.text.trim();
    final next = _newPasswordCtrl.text;
    final confirm = _confirmPasswordCtrl.text;

    if (current.isEmpty || next.isEmpty || confirm.isEmpty) {
      setState(() => _passwordError = 'All fields are required');
      return;
    }
    if (next != confirm) {
      setState(() => _passwordError = 'Passwords do not match');
      return;
    }
    if (next.length < 6) {
      setState(() => _passwordError = 'Password must be at least 6 characters');
      return;
    }

    setState(() {
      _changingPassword = true;
      _passwordError = null;
      _passwordSuccess = false;
    });

    final api = ref.read(apiClientProvider);
    if (api == null) {
      setState(() {
        _changingPassword = false;
        _passwordError = 'Not connected';
      });
      return;
    }

    try {
      await api.put('/auth/password', data: {
        'current_password': current,
        'new_password': next,
      });
      setState(() {
        _changingPassword = false;
        _passwordSuccess = true;
        _passwordError = null;
      });
      _currentPasswordCtrl.clear();
      _newPasswordCtrl.clear();
      _confirmPasswordCtrl.clear();
    } catch (e) {
      setState(() {
        _changingPassword = false;
        _passwordError = _extractError(e);
      });
    }
  }

  String _extractError(Object e) {
    final str = e.toString();
    if (str.contains('403') || str.contains('Forbidden')) {
      return 'Current password is incorrect';
    }
    if (str.contains('400')) return 'Invalid request';
    return 'Failed to change password';
  }

  Future<void> _showAddUserDialog() async {
    final usernameCtrl = TextEditingController();
    final passwordCtrl = TextEditingController();
    String selectedRole = 'viewer';
    String? dialogError;
    bool saving = false;

    await showDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          backgroundColor: NvrColors.bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: const BorderSide(color: NvrColors.border),
          ),
          title: Text('ADD USER', style: NvrTypography.monoSection),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              _HudTextField(controller: usernameCtrl, label: 'USERNAME'),
              const SizedBox(height: 12),
              _HudTextField(
                controller: passwordCtrl,
                label: 'PASSWORD',
                obscure: true,
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<String>(
                initialValue: selectedRole,
                dropdownColor: NvrColors.bgTertiary,
                style: NvrTypography.monoData,
                decoration: _hudInputDecoration('ROLE'),
                items: const [
                  DropdownMenuItem(value: 'viewer', child: Text('VIEWER')),
                  DropdownMenuItem(value: 'admin', child: Text('ADMIN')),
                ],
                onChanged: (v) =>
                    setDialogState(() => selectedRole = v ?? 'viewer'),
              ),
              if (dialogError != null) ...[
                const SizedBox(height: 10),
                Text(
                  dialogError!,
                  style: NvrTypography.body.copyWith(color: NvrColors.danger),
                ),
              ],
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text(
                'CANCEL',
                style: NvrTypography.monoLabel
                    .copyWith(color: NvrColors.textSecondary),
              ),
            ),
            HudButton(
              label: saving ? 'CREATING…' : 'CREATE',
              onPressed: saving
                  ? null
                  : () async {
                      final uname = usernameCtrl.text.trim();
                      final pw = passwordCtrl.text;
                      if (uname.isEmpty || pw.isEmpty) {
                        setDialogState(
                            () => dialogError = 'All fields required');
                        return;
                      }
                      setDialogState(() => saving = true);
                      final api = ref.read(apiClientProvider);
                      try {
                        await api?.post('/users', data: {
                          'username': uname,
                          'password': pw,
                          'role': selectedRole,
                        });
                        ref.invalidate(usersProvider);
                        if (ctx.mounted) Navigator.pop(ctx);
                      } catch (e) {
                        setDialogState(() {
                          saving = false;
                          dialogError = 'Failed to create user';
                        });
                      }
                    },
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showEditUserDialog(User user) async {
    String selectedRole = user.role;
    String? dialogError;
    bool saving = false;

    await showDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          backgroundColor: NvrColors.bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: const BorderSide(color: NvrColors.border),
          ),
          title: Text(
            'EDIT ${user.username.toUpperCase()}',
            style: NvrTypography.monoSection,
          ),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              DropdownButtonFormField<String>(
                initialValue: selectedRole,
                dropdownColor: NvrColors.bgTertiary,
                style: NvrTypography.monoData,
                decoration: _hudInputDecoration('ROLE'),
                items: const [
                  DropdownMenuItem(value: 'viewer', child: Text('VIEWER')),
                  DropdownMenuItem(value: 'admin', child: Text('ADMIN')),
                ],
                onChanged: (v) =>
                    setDialogState(() => selectedRole = v ?? user.role),
              ),
              if (dialogError != null) ...[
                const SizedBox(height: 10),
                Text(
                  dialogError!,
                  style: NvrTypography.body.copyWith(color: NvrColors.danger),
                ),
              ],
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text(
                'CANCEL',
                style: NvrTypography.monoLabel
                    .copyWith(color: NvrColors.textSecondary),
              ),
            ),
            HudButton(
              label: saving ? 'SAVING…' : 'SAVE',
              onPressed: saving
                  ? null
                  : () async {
                      setDialogState(() => saving = true);
                      final api = ref.read(apiClientProvider);
                      try {
                        await api?.put('/users/${user.id}',
                            data: {'role': selectedRole});
                        ref.invalidate(usersProvider);
                        if (ctx.mounted) Navigator.pop(ctx);
                      } catch (e) {
                        setDialogState(() {
                          saving = false;
                          dialogError = 'Failed to update user';
                        });
                      }
                    },
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _deleteUser(User user) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(4),
          side: const BorderSide(color: NvrColors.border),
        ),
        title: Text('DELETE USER', style: NvrTypography.monoSection),
        content: Text(
          'Delete "${user.username}"? This cannot be undone.',
          style: NvrTypography.body,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(
              'CANCEL',
              style:
                  NvrTypography.monoLabel.copyWith(color: NvrColors.textMuted),
            ),
          ),
          HudButton(
            label: 'DELETE',
            style: HudButtonStyle.danger,
            onPressed: () => Navigator.pop(ctx, true),
          ),
        ],
      ),
    );

    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    try {
      await api?.delete('/users/${user.id}');
      ref.invalidate(usersProvider);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'User "${user.username}" deleted',
              style: NvrTypography.monoData,
            ),
            backgroundColor: NvrColors.bgSecondary,
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Failed to delete user',
              style: NvrTypography.monoData.copyWith(color: NvrColors.danger),
            ),
            backgroundColor: NvrColors.bgSecondary,
          ),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authProvider);
    final isAdmin = auth.user?.role == 'admin';
    final usersAsync = ref.watch(usersProvider);

    return Stack(
      children: [
        ListView(
          padding: const EdgeInsets.all(20),
          children: [
            // ── Change Password ──
            Text('CHANGE PASSWORD', style: NvrTypography.monoSection),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                border: Border.all(color: NvrColors.border),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Column(
                children: [
                  _PasswordField(
                    controller: _currentPasswordCtrl,
                    label: 'CURRENT PASSWORD',
                    show: _showCurrentPw,
                    onToggle: () =>
                        setState(() => _showCurrentPw = !_showCurrentPw),
                  ),
                  const SizedBox(height: 12),
                  _PasswordField(
                    controller: _newPasswordCtrl,
                    label: 'NEW PASSWORD',
                    show: _showNewPw,
                    onToggle: () => setState(() => _showNewPw = !_showNewPw),
                  ),
                  const SizedBox(height: 12),
                  _PasswordField(
                    controller: _confirmPasswordCtrl,
                    label: 'CONFIRM NEW PASSWORD',
                    show: _showConfirmPw,
                    onToggle: () =>
                        setState(() => _showConfirmPw = !_showConfirmPw),
                  ),
                  if (_passwordError != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      _passwordError!,
                      style:
                          NvrTypography.body.copyWith(color: NvrColors.danger),
                    ),
                  ],
                  if (_passwordSuccess) ...[
                    const SizedBox(height: 8),
                    Row(
                      children: [
                        const Icon(Icons.check_circle,
                            color: NvrColors.success, size: 14),
                        const SizedBox(width: 6),
                        Text(
                          'Password changed successfully',
                          style: NvrTypography.monoData.copyWith(
                            color: NvrColors.success,
                          ),
                        ),
                      ],
                    ),
                  ],
                  const SizedBox(height: 16),
                  SizedBox(
                    width: double.infinity,
                    child: _changingPassword
                        ? const Center(
                            child: SizedBox(
                              width: 16,
                              height: 16,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: NvrColors.accent,
                              ),
                            ),
                          )
                        : HudButton(
                            label: 'SAVE PASSWORD',
                            onPressed: _changePassword,
                          ),
                  ),
                ],
              ),
            ),

            // ── User list (admin only) ──
            if (isAdmin) ...[
              const SizedBox(height: 28),
              Text('USERS', style: NvrTypography.monoSection),
              const SizedBox(height: 12),
              usersAsync.when(
                loading: () => const Center(
                  child:
                      CircularProgressIndicator(color: NvrColors.accent),
                ),
                error: (e, _) => Text(
                  'Failed to load users: $e',
                  style: NvrTypography.body.copyWith(color: NvrColors.danger),
                ),
                data: (users) => users.isEmpty
                    ? Center(
                        child: Text(
                          'No users found',
                          style: NvrTypography.body,
                        ),
                      )
                    : Column(
                        children: users
                            .map((u) => _UserTile(
                                  user: u,
                                  onEdit: () => _showEditUserDialog(u),
                                  onDelete: () => _deleteUser(u),
                                ))
                            .toList(),
                      ),
              ),
              const SizedBox(height: 80), // FAB clearance
            ],
          ],
        ),
        // FAB for adding users (admin only)
        if (isAdmin)
          Positioned(
            bottom: 16,
            right: 16,
            child: HudButton(
              label: 'ADD USER',
              icon: Icons.person_add,
              onPressed: _showAddUserDialog,
            ),
          ),
      ],
    );
  }
}

// ─────────────────────────────────────────────
// Helper widgets
// ─────────────────────────────────────────────

class _PasswordField extends StatelessWidget {
  final TextEditingController controller;
  final String label;
  final bool show;
  final VoidCallback onToggle;

  const _PasswordField({
    required this.controller,
    required this.label,
    required this.show,
    required this.onToggle,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: !show,
      style: NvrTypography.monoData,
      decoration: _hudInputDecoration(label).copyWith(
        suffixIcon: IconButton(
          icon: Icon(
            show ? Icons.visibility_off : Icons.visibility,
            color: NvrColors.textMuted,
            size: 16,
          ),
          onPressed: onToggle,
        ),
      ),
    );
  }
}

class _UserTile extends StatelessWidget {
  final User user;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  const _UserTile(
      {required this.user, required this.onEdit, required this.onDelete});

  String _initials(String username) {
    if (username.isEmpty) return '?';
    final parts = username.split(RegExp(r'[\s_\-]+'));
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return username
        .substring(0, username.length >= 2 ? 2 : 1)
        .toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    final isAdmin = user.role == 'admin';

    return Dismissible(
      key: ValueKey(user.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        decoration: BoxDecoration(
          color: NvrColors.danger.withOpacity(0.15),
          borderRadius: BorderRadius.circular(4),
        ),
        child: const Icon(Icons.delete, color: NvrColors.danger, size: 18),
      ),
      confirmDismiss: (direction) async {
        onDelete();
        return false;
      },
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: NvrColors.bgSecondary,
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Row(
          children: [
            // Avatar
            Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: NvrColors.accent.withOpacity(0.15),
                borderRadius: BorderRadius.circular(4),
              ),
              alignment: Alignment.center,
              child: Text(
                _initials(user.username),
                style: NvrTypography.monoData.copyWith(
                  color: NvrColors.accent,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    user.username,
                    style: NvrTypography.monoData.copyWith(
                      color: NvrColors.textPrimary,
                    ),
                  ),
                  if (user.cameraPermissions != '*')
                    Text(
                      'Cameras: ${user.cameraPermissions}',
                      style: NvrTypography.monoLabel,
                    ),
                ],
              ),
            ),
            // Role badge
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: isAdmin
                    ? NvrColors.accent.withOpacity(0.12)
                    : NvrColors.bgTertiary,
                border: Border.all(
                  color: isAdmin
                      ? NvrColors.accent.withOpacity(0.3)
                      : NvrColors.border,
                ),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                user.role.toUpperCase(),
                style: NvrTypography.monoLabel.copyWith(
                  color: isAdmin ? NvrColors.accent : NvrColors.textSecondary,
                ),
              ),
            ),
            const SizedBox(width: 8),
            IconButton(
              icon: const Icon(Icons.edit_outlined,
                  color: NvrColors.textMuted, size: 16),
              onPressed: onEdit,
              tooltip: 'Edit',
              padding: EdgeInsets.zero,
              constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
            ),
          ],
        ),
      ),
    );
  }
}

class _HudTextField extends StatelessWidget {
  final TextEditingController controller;
  final String label;
  final bool obscure;

  const _HudTextField({
    required this.controller,
    required this.label,
    this.obscure = false,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: obscure,
      style: NvrTypography.monoData,
      decoration: _hudInputDecoration(label),
    );
  }
}

InputDecoration _hudInputDecoration(String label) {
  return InputDecoration(
    labelText: label,
    labelStyle: NvrTypography.monoLabel,
    filled: true,
    fillColor: NvrColors.bgTertiary,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.accent),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
  );
}
