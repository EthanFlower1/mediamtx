// KAI-312 — Event detail page shell.
//
// Minimal shell that consumes KAI-303's EventDetailsLoader when it exists.
// Until that loader lands, this page shows a static "not available" message
// with the event ID for debugging. Once KAI-303 ships
// `lib/notifications/event_details_loader.dart`, wire it here via an adapter
// rather than re-implementing fetch logic.
//
// Navigation: pushed by [EventsListPage._openDetail] with the event ID and
// tenant ID from the tapped [EventSummary] row. The tenant ID is re-checked
// here as a second cross-tenant guard (belt-and-braces).

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import 'events_strings.dart';

/// Provider for an optional event-detail loader. When KAI-303's
/// EventDetailsLoader ships, override this with a real implementation.
/// The loader takes an event ID and returns a Future of a detail widget.
///
/// Signature contract (adapter target):
///   Future<Widget> call(String eventId, String tenantId)
///
/// If KAI-303's actual signature differs, write a thin adapter — do NOT
/// modify EventDetailsLoader.
final eventDetailLoaderProvider =
    Provider<Future<Widget> Function(String eventId, String tenantId)?>(
        (ref) => null);

class EventDetailPage extends ConsumerWidget {
  final String eventId;
  final String tenantId;

  const EventDetailPage({
    super.key,
    required this.eventId,
    required this.tenantId,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Cross-tenant guard (defense-in-depth, second check after list page).
    final session = ref.watch(appSessionProvider);
    if (session.tenantRef != tenantId) {
      debugPrint(
        'CrossTenantEventViolation: detail page blocked '
        '(session=${session.tenantRef} event=$tenantId eventId=$eventId)',
      );
      return Scaffold(
        appBar: AppBar(title: Text(EventsStrings.detailTitle)),
        body: const Center(child: Text('Access denied.')),
      );
    }

    final loader = ref.watch(eventDetailLoaderProvider);
    return Scaffold(
      appBar: AppBar(title: Text(EventsStrings.detailTitle)),
      body: loader != null
          ? FutureBuilder<Widget>(
              future: loader(eventId, tenantId),
              builder: (ctx, snap) {
                if (snap.connectionState == ConnectionState.waiting) {
                  return Center(child: Text(EventsStrings.detailLoading));
                }
                if (snap.hasError) {
                  return Center(
                    child: Text('Error: ${snap.error}'),
                  );
                }
                return snap.data ?? const SizedBox.shrink();
              },
            )
          : Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(EventsStrings.detailNotAvailable),
                    const SizedBox(height: 8),
                    SelectableText(
                      'Event ID: $eventId',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ),
    );
  }
}
