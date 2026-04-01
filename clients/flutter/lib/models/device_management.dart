class DateTimeInfo {
  final String type; // "Manual" or "NTP"
  final bool daylightSaving;
  final String timezone;
  final String utcTime;
  final String localTime;

  const DateTimeInfo({
    required this.type,
    required this.daylightSaving,
    required this.timezone,
    required this.utcTime,
    required this.localTime,
  });

  factory DateTimeInfo.fromJson(Map<String, dynamic> json) => DateTimeInfo(
        type: json['type'] as String? ?? '',
        daylightSaving: json['daylight_saving'] as bool? ?? false,
        timezone: json['timezone'] as String? ?? '',
        utcTime: json['utc_time'] as String? ?? '',
        localTime: json['local_time'] as String? ?? '',
      );
}

class HostnameInfo {
  final bool fromDHCP;
  final String name;

  const HostnameInfo({required this.fromDHCP, required this.name});

  factory HostnameInfo.fromJson(Map<String, dynamic> json) => HostnameInfo(
        fromDHCP: json['from_dhcp'] as bool? ?? false,
        name: json['name'] as String? ?? '',
      );
}

class NetworkInterfaceInfo {
  final String token;
  final bool enabled;
  final String mac;
  final IPv4Config? ipv4;

  const NetworkInterfaceInfo({
    required this.token,
    required this.enabled,
    required this.mac,
    this.ipv4,
  });

  factory NetworkInterfaceInfo.fromJson(Map<String, dynamic> json) =>
      NetworkInterfaceInfo(
        token: json['token'] as String? ?? '',
        enabled: json['enabled'] as bool? ?? false,
        mac: json['mac'] as String? ?? '',
        ipv4: json['ipv4'] != null
            ? IPv4Config.fromJson(json['ipv4'] as Map<String, dynamic>)
            : null,
      );
}

class IPv4Config {
  final bool enabled;
  final bool dhcp;
  final String address;
  final int prefix;

  const IPv4Config({
    required this.enabled,
    required this.dhcp,
    required this.address,
    required this.prefix,
  });

  factory IPv4Config.fromJson(Map<String, dynamic> json) => IPv4Config(
        enabled: json['enabled'] as bool? ?? false,
        dhcp: json['dhcp'] as bool? ?? false,
        address: json['address'] as String? ?? '',
        prefix: json['prefix_length'] as int? ?? 0,
      );
}

class NetworkProtocolInfo {
  final String name;
  final bool enabled;
  final int port;

  const NetworkProtocolInfo({
    required this.name,
    required this.enabled,
    required this.port,
  });

  factory NetworkProtocolInfo.fromJson(Map<String, dynamic> json) =>
      NetworkProtocolInfo(
        name: json['name'] as String? ?? '',
        enabled: json['enabled'] as bool? ?? false,
        port: json['port'] as int? ?? 0,
      );

  Map<String, dynamic> toJson() => {
        'name': name,
        'enabled': enabled,
        'port': port,
      };
}

class DeviceUser {
  final String username;
  final String role; // Administrator, Operator, User

  const DeviceUser({required this.username, required this.role});

  factory DeviceUser.fromJson(Map<String, dynamic> json) => DeviceUser(
        username: json['username'] as String? ?? '',
        role: json['role'] as String? ?? '',
      );
}
