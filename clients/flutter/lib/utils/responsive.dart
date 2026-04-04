import 'package:flutter/widgets.dart';

/// Device form-factor breakpoints used throughout the NVR client.
///
/// - **phone**:   width < 600
/// - **tablet**:  600 <= width < 1200
/// - **desktop**: width >= 1200
enum DeviceType { phone, tablet, desktop }

class Responsive {
  Responsive._();

  static const double phoneMax = 600;
  static const double tabletMax = 1200;

  /// Determine the [DeviceType] from a raw width value.
  static DeviceType deviceType(double width) {
    if (width < phoneMax) return DeviceType.phone;
    if (width < tabletMax) return DeviceType.tablet;
    return DeviceType.desktop;
  }

  /// Convenience: get [DeviceType] from a [BuildContext].
  static DeviceType of(BuildContext context) {
    return deviceType(MediaQuery.of(context).size.width);
  }

  static bool isPhone(BuildContext context) => of(context) == DeviceType.phone;
  static bool isTablet(BuildContext context) => of(context) == DeviceType.tablet;
  static bool isDesktop(BuildContext context) => of(context) == DeviceType.desktop;
}
