// KAI-297 — Background token refresh scheduler.
//
// Per spec: refresh ~5 minutes before expiry, run from `WorkManager` on
// Android and `BGTaskScheduler` on iOS. The platform plumbing is a stub here
// (we don't ship the platform channels in this PR) but the *Dart-side*
// scheduling logic is the load-bearing piece — it computes when to fire and
// drives the refresh through `LoginService.refresh`.
//
// The class is structured so a real WorkManager / BGTaskScheduler binding can
// later inject itself by overriding [BackgroundTaskBinding]. Tests use the
// in-memory binding to advance virtual time and assert refresh fired exactly
// when expected.
//
// Threading model: this scheduler is single-shot per session. The caller
// (typically the AppSession refresh orchestrator) re-arms it after each
// successful refresh with the new `expiresAt`.

import 'dart:async';

import '../state/app_session.dart';
import 'auth_types.dart';
import 'login_service.dart';

/// Buffer ahead of token expiry. Five minutes matches the spec's
/// "refresh 5 min before expiry" rule.
const Duration kRefreshLeadTime = Duration(minutes: 5);

/// Floor on the scheduled delay. If the access token is already past its
/// lead-time window, we still wait at least this long so we don't hammer the
/// server in a tight loop.
const Duration kRefreshMinDelay = Duration(seconds: 1);

/// Pluggable seam for "schedule a callback to run in the future". Production
/// wires this to WorkManager (Android) / BGTaskScheduler (iOS); tests use
/// [InMemoryBackgroundTaskBinding] which lets `fake_async` drive virtual time.
abstract class BackgroundTaskBinding {
  /// Schedule [callback] to run after [delay]. Returns a handle the caller can
  /// use to cancel.
  BackgroundTaskHandle schedule(Duration delay, Future<void> Function() callback);
}

abstract class BackgroundTaskHandle {
  void cancel();
  bool get isCancelled;
}

/// Default in-process binding. Uses `Timer.periodic`-equivalent semantics via
/// a one-shot `Timer`. Suitable for foreground sessions and unit tests; on
/// Android/iOS background, swap in a real WorkManager / BGTaskScheduler
/// adapter (a separate KAI ticket lands the platform channels).
class InMemoryBackgroundTaskBinding implements BackgroundTaskBinding {
  @override
  BackgroundTaskHandle schedule(
    Duration delay,
    Future<void> Function() callback,
  ) {
    final handle = _TimerHandle();
    handle._timer = Timer(delay, () async {
      if (handle.isCancelled) return;
      try {
        await callback();
      } catch (_) {
        // Errors are surfaced through the LoginError pathway from the caller's
        // refresh task; the binding itself never throws.
      }
    });
    return handle;
  }
}

class _TimerHandle implements BackgroundTaskHandle {
  Timer? _timer;
  bool _cancelled = false;

  @override
  void cancel() {
    _cancelled = true;
    _timer?.cancel();
  }

  @override
  bool get isCancelled => _cancelled;
}

/// Outcome of a refresh attempt. The caller decides what to do with it —
/// typically `success` triggers `AppSessionNotifier.setTokens`, while
/// `expired` bounces the user back to login.
sealed class RefreshOutcome {
  const RefreshOutcome();
}

class RefreshSuccess extends RefreshOutcome {
  final LoginResult result;
  const RefreshSuccess(this.result);
}

class RefreshFailure extends RefreshOutcome {
  final LoginError error;
  const RefreshFailure(this.error);
}

/// Schedules and executes background refresh of an [AppSession]'s access
/// token. The scheduler is one-shot — re-arm it after each refresh so the
/// next deadline reflects the freshly issued `expiresAt`.
class RefreshScheduler {
  final LoginService _login;
  final BackgroundTaskBinding _binding;
  final DateTime Function() _now;

  BackgroundTaskHandle? _pending;

  RefreshScheduler({
    required LoginService loginService,
    BackgroundTaskBinding? binding,
    DateTime Function()? now,
  })  : _login = loginService,
        _binding = binding ?? InMemoryBackgroundTaskBinding(),
        _now = now ?? DateTime.now;

  /// Compute the delay before we should refresh. Exposed as a static so tests
  /// can assert the math without scheduling anything.
  static Duration computeDelay({
    required DateTime expiresAt,
    required DateTime now,
    Duration leadTime = kRefreshLeadTime,
    Duration minDelay = kRefreshMinDelay,
  }) {
    final raw = expiresAt.difference(now) - leadTime;
    return raw < minDelay ? minDelay : raw;
  }

  /// Arm the scheduler against [session]. The supplied [onOutcome] callback
  /// fires once with the refresh outcome. The caller is responsible for re-
  /// arming after success.
  ///
  /// Throws [StateError] if the session has no active connection or no
  /// refresh token — those cases should bounce the user to login, not run a
  /// background refresh.
  void arm({
    required AppSession session,
    required DateTime expiresAt,
    required void Function(RefreshOutcome) onOutcome,
  }) {
    if (session.activeConnection == null) {
      throw StateError('RefreshScheduler.arm: no active connection');
    }
    if (session.refreshToken == null || session.refreshToken!.isEmpty) {
      throw StateError('RefreshScheduler.arm: no refresh token');
    }
    cancel();
    final delay = computeDelay(expiresAt: expiresAt, now: _now());
    _pending = _binding.schedule(delay, () async {
      try {
        final result = await _login.refresh(session);
        onOutcome(RefreshSuccess(result));
      } on LoginError catch (e) {
        onOutcome(RefreshFailure(e));
      }
    });
  }

  /// Cancel any pending refresh. Safe to call multiple times.
  void cancel() {
    _pending?.cancel();
    _pending = null;
  }

  /// Whether a refresh is currently armed.
  bool get isArmed => _pending != null && !_pending!.isCancelled;
}
