import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/events/events_model.dart';
import 'package:nvr_client/proto_adapters/events_proto_adapter.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/directory_ingest.pb.dart';

void main() {
  group('eventSummaryFromProto', () {
    test('maps basic AI event to EventSummary', () {
      final ts = DateTime(2026, 4, 8, 12, 0);
      final pb = PbAIEvent(
        eventId: 'evt-1',
        cameraId: 'cam-1',
        kind: PbAIEventKind.person,
        observedAt: ts,
      );

      final summary = eventSummaryFromProto(
        pb,
        tenantId: 'tenant-abc',
        cameraNameLookup: (id) => 'Front Door',
      );

      expect(summary.id, 'evt-1');
      expect(summary.cameraId, 'cam-1');
      expect(summary.cameraName, 'Front Door');
      expect(summary.kind, 'person');
      expect(summary.timestamp, ts);
      expect(summary.severity, EventSeverity.info);
      expect(summary.tenantId, 'tenant-abc');
    });

    test('uses cameraId as fallback name', () {
      final pb = PbAIEvent(
        eventId: 'evt-2',
        cameraId: 'cam-unknown',
        kind: PbAIEventKind.motion,
        observedAt: DateTime.now(),
      );

      final summary = eventSummaryFromProto(pb, tenantId: 't1');
      expect(summary.cameraName, 'cam-unknown');
    });

    test('tamper is critical severity', () {
      final pb = PbAIEvent(
        eventId: 'evt-3',
        cameraId: 'cam-1',
        kind: PbAIEventKind.tamper,
        observedAt: DateTime.now(),
      );
      expect(
        eventSummaryFromProto(pb, tenantId: 't1').severity,
        EventSeverity.critical,
      );
    });

    test('audio alarm is critical severity', () {
      final pb = PbAIEvent(
        eventId: 'evt-4',
        cameraId: 'cam-1',
        kind: PbAIEventKind.audioAlarm,
        observedAt: DateTime.now(),
      );
      expect(
        eventSummaryFromProto(pb, tenantId: 't1').severity,
        EventSeverity.critical,
      );
    });

    test('line crossing is warning severity', () {
      final pb = PbAIEvent(
        eventId: 'evt-5',
        cameraId: 'cam-1',
        kind: PbAIEventKind.lineCrossing,
        observedAt: DateTime.now(),
      );
      expect(
        eventSummaryFromProto(pb, tenantId: 't1').severity,
        EventSeverity.warning,
      );
    });

    test('custom kind label overrides default mapping', () {
      final pb = PbAIEvent(
        eventId: 'evt-6',
        cameraId: 'cam-1',
        kind: PbAIEventKind.unspecified,
        kindLabel: 'package_detected',
        observedAt: DateTime.now(),
      );
      expect(
        eventSummaryFromProto(pb, tenantId: 't1').kind,
        'package_detected',
      );
    });

    test('eventSummariesFromProto converts list', () {
      final events = eventSummariesFromProto(
        [
          PbAIEvent(eventId: 'a', cameraId: 'c1', kind: PbAIEventKind.motion, observedAt: DateTime.now()),
          PbAIEvent(eventId: 'b', cameraId: 'c2', kind: PbAIEventKind.vehicle, observedAt: DateTime.now()),
        ],
        tenantId: 't1',
      );
      expect(events.length, 2);
      expect(events[0].id, 'a');
      expect(events[1].kind, 'vehicle');
    });
  });
}
