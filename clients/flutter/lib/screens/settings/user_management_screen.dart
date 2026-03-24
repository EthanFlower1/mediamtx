import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/user.dart';
import '../../providers/auth_provider.dart';
import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';

class UserManagementScreen extends ConsumerStatefulWidget {
  const UserManagementScreen({super.key});

  @override
  ConsumerState<UserManagementScreen> createState() => _UserManagementScreenState();
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
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          title: const Text('Add User', style: TextStyle(color: NvrColors.textPrimary)),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              _DialogTextField(controller: usernameCtrl, label: 'Username'),
              const SizedBox(height: 12),
              _DialogTextField(
                controller: passwordCtrl,
                label: 'Password',
                obscure: true,
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<String>(
                initialValue: selectedRole,
                dropdownColor: NvrColors.bgTertiary,
                style: const TextStyle(color: NvrColors.textPrimary),
                decoration: _inputDecoration('Role'),
                items: const [
                  DropdownMenuItem(value: 'viewer', child: Text('Viewer')),
                  DropdownMenuItem(value: 'admin', child: Text('Admin')),
                ],
                onChanged: (v) => setDialogState(() => selectedRole = v ?? 'viewer'),
              ),
              if (dialogError != null) ...[
                const SizedBox(height: 10),
                Text(
                  dialogError!,
                  style: const TextStyle(color: NvrColors.danger, fontSize: 13),
                ),
              ],
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Cancel', style: TextStyle(color: NvrColors.textMuted)),
            ),
            ElevatedButton(
              onPressed: saving
                  ? null
                  : () async {
                      final uname = usernameCtrl.text.trim();
                      final pw = passwordCtrl.text;
                      if (uname.isEmpty || pw.isEmpty) {
                        setDialogState(() => dialogError = 'All fields required');
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
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
              ),
              child: saving
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: Colors.white,
                      ),
                    )
                  : const Text('Create'),
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
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          title: Text(
            'Edit ${user.username}',
            style: const TextStyle(color: NvrColors.textPrimary),
          ),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              DropdownButtonFormField<String>(
                initialValue: selectedRole,
                dropdownColor: NvrColors.bgTertiary,
                style: const TextStyle(color: NvrColors.textPrimary),
                decoration: _inputDecoration('Role'),
                items: const [
                  DropdownMenuItem(value: 'viewer', child: Text('Viewer')),
                  DropdownMenuItem(value: 'admin', child: Text('Admin')),
                ],
                onChanged: (v) => setDialogState(() => selectedRole = v ?? user.role),
              ),
              if (dialogError != null) ...[
                const SizedBox(height: 10),
                Text(
                  dialogError!,
                  style: const TextStyle(color: NvrColors.danger, fontSize: 13),
                ),
              ],
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Cancel', style: TextStyle(color: NvrColors.textMuted)),
            ),
            ElevatedButton(
              onPressed: saving
                  ? null
                  : () async {
                      setDialogState(() => saving = true);
                      final api = ref.read(apiClientProvider);
                      try {
                        await api?.put('/users/${user.id}', data: {
                          'role': selectedRole,
                        });
                        ref.invalidate(usersProvider);
                        if (ctx.mounted) Navigator.pop(ctx);
                      } catch (e) {
                        setDialogState(() {
                          saving = false;
                          dialogError = 'Failed to update user';
                        });
                      }
                    },
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
              ),
              child: const Text('Save'),
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
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        title: const Text('Delete User', style: TextStyle(color: NvrColors.textPrimary)),
        content: Text(
          'Are you sure you want to delete "${user.username}"?',
          style: const TextStyle(color: NvrColors.textSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel', style: TextStyle(color: NvrColors.textMuted)),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.danger,
              foregroundColor: Colors.white,
            ),
            child: const Text('Delete'),
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
            content: Text('User "${user.username}" deleted'),
            backgroundColor: NvrColors.success,
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Failed to delete user'),
            backgroundColor: NvrColors.danger,
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
          padding: const EdgeInsets.all(16),
          children: [
            // --- Change Password section ---
            const _SectionHeader(title: 'Change Password', icon: Icons.lock_outline),
            const SizedBox(height: 12),
            Card(
              color: NvrColors.bgSecondary,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
                side: const BorderSide(color: NvrColors.border),
              ),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    _PasswordField(
                      controller: _currentPasswordCtrl,
                      label: 'Current Password',
                      show: _showCurrentPw,
                      onToggle: () => setState(() => _showCurrentPw = !_showCurrentPw),
                    ),
                    const SizedBox(height: 12),
                    _PasswordField(
                      controller: _newPasswordCtrl,
                      label: 'New Password',
                      show: _showNewPw,
                      onToggle: () => setState(() => _showNewPw = !_showNewPw),
                    ),
                    const SizedBox(height: 12),
                    _PasswordField(
                      controller: _confirmPasswordCtrl,
                      label: 'Confirm New Password',
                      show: _showConfirmPw,
                      onToggle: () => setState(() => _showConfirmPw = !_showConfirmPw),
                    ),
                    if (_passwordError != null) ...[
                      const SizedBox(height: 8),
                      Text(
                        _passwordError!,
                        style: const TextStyle(color: NvrColors.danger, fontSize: 13),
                      ),
                    ],
                    if (_passwordSuccess) ...[
                      const SizedBox(height: 8),
                      const Row(
                        children: [
                          Icon(Icons.check_circle, color: NvrColors.success, size: 16),
                          SizedBox(width: 6),
                          Text(
                            'Password changed successfully',
                            style: TextStyle(color: NvrColors.success, fontSize: 13),
                          ),
                        ],
                      ),
                    ],
                    const SizedBox(height: 16),
                    SizedBox(
                      width: double.infinity,
                      child: ElevatedButton(
                        onPressed: _changingPassword ? null : _changePassword,
                        style: ElevatedButton.styleFrom(
                          backgroundColor: NvrColors.accent,
                          foregroundColor: Colors.white,
                          padding: const EdgeInsets.symmetric(vertical: 12),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(8),
                          ),
                        ),
                        child: _changingPassword
                            ? const SizedBox(
                                width: 16,
                                height: 16,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                  color: Colors.white,
                                ),
                              )
                            : const Text('Save Password'),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            // --- User list (admin only) ---
            if (isAdmin) ...[
              const SizedBox(height: 24),
              const _SectionHeader(title: 'Users', icon: Icons.group_outlined),
              const SizedBox(height: 12),
              usersAsync.when(
                loading: () => const Center(child: CircularProgressIndicator()),
                error: (e, _) => Text(
                  'Failed to load users: $e',
                  style: const TextStyle(color: NvrColors.danger),
                ),
                data: (users) => users.isEmpty
                    ? const Center(
                        child: Text(
                          'No users found',
                          style: TextStyle(color: NvrColors.textMuted),
                        ),
                      )
                    : Column(
                        children: users.map((u) => _UserTile(
                          user: u,
                          onEdit: () => _showEditUserDialog(u),
                          onDelete: () => _deleteUser(u),
                        )).toList(),
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
            child: FloatingActionButton.extended(
              onPressed: _showAddUserDialog,
              backgroundColor: NvrColors.accent,
              foregroundColor: Colors.white,
              icon: const Icon(Icons.person_add),
              label: const Text('Add User'),
            ),
          ),
      ],
    );
  }
}

// ────────────────────────────────────────────────────────────────
// Helper widgets
// ────────────────────────────────────────────────────────────────

class _SectionHeader extends StatelessWidget {
  final String title;
  final IconData icon;

  const _SectionHeader({required this.title, required this.icon});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(icon, color: NvrColors.accent, size: 18),
        const SizedBox(width: 8),
        Text(
          title,
          style: const TextStyle(
            color: NvrColors.textSecondary,
            fontSize: 13,
            fontWeight: FontWeight.w600,
            letterSpacing: 0.5,
          ),
        ),
      ],
    );
  }
}

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
      style: const TextStyle(color: NvrColors.textPrimary, fontSize: 14),
      decoration: InputDecoration(
        labelText: label,
        labelStyle: const TextStyle(color: NvrColors.textMuted, fontSize: 13),
        filled: true,
        fillColor: NvrColors.bgInput,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.accent),
        ),
        suffixIcon: IconButton(
          icon: Icon(
            show ? Icons.visibility_off : Icons.visibility,
            color: NvrColors.textMuted,
            size: 18,
          ),
          onPressed: onToggle,
        ),
        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      ),
    );
  }
}

class _UserTile extends StatelessWidget {
  final User user;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  const _UserTile({required this.user, required this.onEdit, required this.onDelete});

  String _initials(String username) {
    if (username.isEmpty) return '?';
    final parts = username.split(RegExp(r'[\s_\-]+'));
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return username.substring(0, username.length >= 2 ? 2 : 1).toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    final isAdmin = user.role == 'admin';
    final badgeColor = isAdmin ? NvrColors.accent : NvrColors.bgTertiary;
    final badgeTextColor = isAdmin ? Colors.white : NvrColors.textSecondary;

    return Dismissible(
      key: ValueKey(user.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        decoration: BoxDecoration(
          color: NvrColors.danger.withValues(alpha: 0.2),
          borderRadius: BorderRadius.circular(10),
        ),
        child: const Icon(Icons.delete, color: NvrColors.danger),
      ),
      confirmDismiss: (direction) async {
        onDelete();
        return false; // deletion handled in callback
      },
      child: Card(
        color: NvrColors.bgSecondary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(10),
          side: const BorderSide(color: NvrColors.border),
        ),
        margin: const EdgeInsets.only(bottom: 8),
        child: ListTile(
          leading: CircleAvatar(
            backgroundColor: NvrColors.accent.withValues(alpha: 0.2),
            child: Text(
              _initials(user.username),
              style: const TextStyle(
                color: NvrColors.accent,
                fontWeight: FontWeight.w700,
                fontSize: 13,
              ),
            ),
          ),
          title: Text(
            user.username,
            style: const TextStyle(
              color: NvrColors.textPrimary,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
          subtitle: user.cameraPermissions != '*'
              ? Text(
                  'Cameras: ${user.cameraPermissions}',
                  style: const TextStyle(color: NvrColors.textMuted, fontSize: 12),
                )
              : null,
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: BoxDecoration(
                  color: badgeColor,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  user.role.toUpperCase(),
                  style: TextStyle(
                    color: badgeTextColor,
                    fontSize: 10,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(width: 4),
              IconButton(
                icon: const Icon(Icons.edit_outlined, color: NvrColors.textMuted, size: 18),
                onPressed: onEdit,
                tooltip: 'Edit',
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _DialogTextField extends StatelessWidget {
  final TextEditingController controller;
  final String label;
  final bool obscure;

  const _DialogTextField({
    required this.controller,
    required this.label,
    this.obscure = false,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: obscure,
      style: const TextStyle(color: NvrColors.textPrimary, fontSize: 14),
      decoration: _inputDecoration(label),
    );
  }
}

InputDecoration _inputDecoration(String label) {
  return InputDecoration(
    labelText: label,
    labelStyle: const TextStyle(color: NvrColors.textMuted, fontSize: 13),
    filled: true,
    fillColor: NvrColors.bgTertiary,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(8),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(8),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(8),
      borderSide: const BorderSide(color: NvrColors.accent),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
  );
}
