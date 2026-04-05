import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/utils/responsive.dart';

void main() {
  group('Responsive.deviceType (from raw width)', () {
    test('width 0 is phone', () {
      expect(Responsive.deviceType(0), DeviceType.phone);
    });

    test('width 320 is phone', () {
      expect(Responsive.deviceType(320), DeviceType.phone);
    });

    test('width 599 is phone', () {
      expect(Responsive.deviceType(599), DeviceType.phone);
    });

    test('width 600 is tablet', () {
      expect(Responsive.deviceType(600), DeviceType.tablet);
    });

    test('width 1199 is tablet', () {
      expect(Responsive.deviceType(1199), DeviceType.tablet);
    });

    test('width 1200 is desktop', () {
      expect(Responsive.deviceType(1200), DeviceType.desktop);
    });

    test('width 1920 is desktop', () {
      expect(Responsive.deviceType(1920), DeviceType.desktop);
    });
  });

  group('Responsive breakpoint constants', () {
    test('phoneMax is 600', () {
      expect(Responsive.phoneMax, 600);
    });

    test('tabletMax is 1200', () {
      expect(Responsive.tabletMax, 1200);
    });
  });

  group('Responsive context helpers', () {
    testWidgets('isPhone returns true for narrow screens', (tester) async {
      tester.view.physicalSize = const Size(599, 800);
      tester.view.devicePixelRatio = 1.0;

      bool? phone;
      bool? tablet;
      bool? desktop;

      await tester.pumpWidget(
        Directionality(
          textDirection: TextDirection.ltr,
          child: MediaQuery(
            data: const MediaQueryData(size: Size(599, 800)),
            child: Builder(builder: (context) {
              phone = Responsive.isPhone(context);
              tablet = Responsive.isTablet(context);
              desktop = Responsive.isDesktop(context);
              return const SizedBox.shrink();
            }),
          ),
        ),
      );

      expect(phone, isTrue);
      expect(tablet, isFalse);
      expect(desktop, isFalse);

      tester.view.resetPhysicalSize();
      tester.view.resetDevicePixelRatio();
    });

    testWidgets('isTablet returns true for medium screens', (tester) async {
      await tester.pumpWidget(
        Directionality(
          textDirection: TextDirection.ltr,
          child: MediaQuery(
            data: const MediaQueryData(size: Size(800, 600)),
            child: Builder(builder: (context) {
              expect(Responsive.isPhone(context), isFalse);
              expect(Responsive.isTablet(context), isTrue);
              expect(Responsive.isDesktop(context), isFalse);
              return const SizedBox.shrink();
            }),
          ),
        ),
      );
    });

    testWidgets('isDesktop returns true for wide screens', (tester) async {
      await tester.pumpWidget(
        Directionality(
          textDirection: TextDirection.ltr,
          child: MediaQuery(
            data: const MediaQueryData(size: Size(1400, 900)),
            child: Builder(builder: (context) {
              expect(Responsive.isPhone(context), isFalse);
              expect(Responsive.isTablet(context), isFalse);
              expect(Responsive.isDesktop(context), isTrue);
              return const SizedBox.shrink();
            }),
          ),
        ),
      );
    });
  });
}
