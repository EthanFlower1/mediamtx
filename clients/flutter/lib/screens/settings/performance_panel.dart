import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:fl_chart/fl_chart.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/snackbar_helper.dart';

class PerformancePanel extends ConsumerStatefulWidget {
  const PerformancePanel({super.key});

  @override
  ConsumerState<PerformancePanel> createState() => _PerformancePanelState();
}

class _PerformancePanelState extends ConsumerState<PerformancePanel> {
  Map<String, dynamic>? _current;
  List<dynamic> _history = [];
  Timer? _refreshTimer;
  bool _loading = true;
  String? _metricsError;

  @override
  void initState() {
    super.initState();
    _fetchMetrics();
    _refreshTimer =
        Timer.periodic(const Duration(seconds: 10), (_) => _fetchMetrics());
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }

  Future<void> _fetchMetrics() async {
    final client = ref.read(apiClientProvider);
    if (client == null) return;

    try {
      final response = await client.get<Map<String, dynamic>>('/system/metrics');
      final data = response.data;
      if (data != null && mounted) {
        setState(() {
          _current = data['current'] as Map<String, dynamic>?;
          _history = (data['history'] as List<dynamic>?) ?? [];
          _loading = false;
          _metricsError = null;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _metricsError = 'Failed to load metrics';
        });
      }
    }
  }

  List<FlSpot> _spots(String key) {
    return _history.asMap().entries.map((e) {
      final val = (e.value[key] as num?)?.toDouble() ?? 0;
      return FlSpot(e.key.toDouble(), val);
    }).toList();
  }

  Widget _kvRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: NvrTypography.monoLabel),
          Text(value, style: NvrTypography.monoData),
        ],
      ),
    );
  }

  Widget _sectionContainer({required Widget child}) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border.all(color: NvrColors.border, width: 1),
        borderRadius: BorderRadius.circular(8),
      ),
      child: child,
    );
  }

  String _formatTimestamp(dynamic ts) {
    if (ts == null) return '';
    try {
      final dt = DateTime.parse(ts.toString()).toLocal();
      final h = dt.hour.toString().padLeft(2, '0');
      final m = dt.minute.toString().padLeft(2, '0');
      return '$h:$m';
    } catch (_) {
      return '';
    }
  }

  /// Returns bottom title text for the chart at the given x index.
  String _bottomLabel(double x, int count) {
    if (_history.isEmpty) return '';
    final idx = x.round().clamp(0, _history.length - 1);
    final entry = _history[idx];
    final ts = entry['timestamp'];
    return _formatTimestamp(ts);
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }

    final cpuSpots = _spots('cpu_percent');
    final memSpots = _spots('mem_percent');
    final heapSpots = _spots('mem_alloc_mb');

    final maxHeap = heapSpots.isEmpty
        ? 1.0
        : heapSpots.map((s) => s.y).reduce((a, b) => a > b ? a : b);
    final heapMax = (maxHeap * 1.2).ceilToDouble().clamp(1.0, double.infinity);

    final count = _history.length;
    // Show ~6 x-axis labels
    final xInterval = count <= 1 ? 1.0 : ((count - 1) / 6.0);

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ── CPU & Memory chart ──
          _sectionContainer(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('CPU & MEMORY USAGE', style: NvrTypography.monoSection),
                const SizedBox(height: 10),
                SizedBox(
                  height: 200,
                  child: cpuSpots.isEmpty
                      ? Center(
                          child: Text(
                            'No data yet',
                            style: NvrTypography.body,
                          ),
                        )
                      : LineChart(
                          LineChartData(
                            backgroundColor: Colors.transparent,
                            gridData: FlGridData(
                              show: true,
                              drawVerticalLine: false,
                              getDrawingHorizontalLine: (_) => FlLine(
                                color: NvrColors.border,
                                strokeWidth: 0.5,
                              ),
                            ),
                            borderData: FlBorderData(show: false),
                            minY: 0,
                            maxY: 100,
                            titlesData: FlTitlesData(
                              leftTitles: AxisTitles(
                                sideTitles: SideTitles(
                                  showTitles: true,
                                  reservedSize: 32,
                                  interval: 25,
                                  getTitlesWidget: (val, _) => Text(
                                    '${val.toInt()}%',
                                    style: NvrTypography.monoLabel,
                                  ),
                                ),
                              ),
                              rightTitles: const AxisTitles(
                                sideTitles: SideTitles(showTitles: false),
                              ),
                              topTitles: const AxisTitles(
                                sideTitles: SideTitles(showTitles: false),
                              ),
                              bottomTitles: AxisTitles(
                                sideTitles: SideTitles(
                                  showTitles: true,
                                  reservedSize: 20,
                                  interval: xInterval,
                                  getTitlesWidget: (val, _) => Text(
                                    _bottomLabel(val, count),
                                    style: NvrTypography.monoLabel,
                                  ),
                                ),
                              ),
                            ),
                            lineTouchData: LineTouchData(
                              touchTooltipData: LineTouchTooltipData(
                                getTooltipColor: (_) => NvrColors.bgTertiary,
                                getTooltipItems: (spots) => spots
                                    .map((s) => LineTooltipItem(
                                          '${s.y.toStringAsFixed(1)}%',
                                          NvrTypography.monoData.copyWith(
                                            color: s.bar.color,
                                          ),
                                        ))
                                    .toList(),
                              ),
                            ),
                            lineBarsData: [
                              LineChartBarData(
                                spots: cpuSpots,
                                color: NvrColors.accent,
                                barWidth: 2,
                                dotData: const FlDotData(show: false),
                                belowBarData: BarAreaData(
                                  show: true,
                                  color: NvrColors.accent.withOpacity(0.1),
                                ),
                              ),
                              LineChartBarData(
                                spots: memSpots,
                                color: const Color(0xFF22c55e),
                                barWidth: 2,
                                dotData: const FlDotData(show: false),
                                belowBarData: BarAreaData(
                                  show: true,
                                  color: const Color(0xFF22c55e)
                                      .withOpacity(0.1),
                                ),
                              ),
                            ],
                          ),
                        ),
                ),
                const SizedBox(height: 8),
                // Legend
                Row(
                  children: [
                    _LegendDot(color: NvrColors.accent),
                    const SizedBox(width: 6),
                    Text('CPU', style: NvrTypography.monoData),
                    const SizedBox(width: 16),
                    const _LegendDot(color: Color(0xFF22c55e)),
                    const SizedBox(width: 6),
                    Text('Memory', style: NvrTypography.monoData),
                  ],
                ),
              ],
            ),
          ),

          const SizedBox(height: 16),

          // ── Process Memory chart ──
          _sectionContainer(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('PROCESS MEMORY', style: NvrTypography.monoSection),
                const SizedBox(height: 10),
                SizedBox(
                  height: 200,
                  child: heapSpots.isEmpty
                      ? Center(
                          child: Text(
                            'No data yet',
                            style: NvrTypography.body,
                          ),
                        )
                      : LineChart(
                          LineChartData(
                            backgroundColor: Colors.transparent,
                            gridData: FlGridData(
                              show: true,
                              drawVerticalLine: false,
                              getDrawingHorizontalLine: (_) => FlLine(
                                color: NvrColors.border,
                                strokeWidth: 0.5,
                              ),
                            ),
                            borderData: FlBorderData(show: false),
                            minY: 0,
                            maxY: heapMax,
                            titlesData: FlTitlesData(
                              leftTitles: AxisTitles(
                                sideTitles: SideTitles(
                                  showTitles: true,
                                  reservedSize: 40,
                                  getTitlesWidget: (val, _) => Text(
                                    '${val.toStringAsFixed(0)}M',
                                    style: NvrTypography.monoLabel,
                                  ),
                                ),
                              ),
                              rightTitles: const AxisTitles(
                                sideTitles: SideTitles(showTitles: false),
                              ),
                              topTitles: const AxisTitles(
                                sideTitles: SideTitles(showTitles: false),
                              ),
                              bottomTitles: AxisTitles(
                                sideTitles: SideTitles(
                                  showTitles: true,
                                  reservedSize: 20,
                                  interval: xInterval,
                                  getTitlesWidget: (val, _) => Text(
                                    _bottomLabel(val, count),
                                    style: NvrTypography.monoLabel,
                                  ),
                                ),
                              ),
                            ),
                            lineTouchData: LineTouchData(
                              touchTooltipData: LineTouchTooltipData(
                                getTooltipColor: (_) => NvrColors.bgTertiary,
                                getTooltipItems: (spots) => spots
                                    .map((s) => LineTooltipItem(
                                          '${s.y.toStringAsFixed(1)} MB',
                                          NvrTypography.monoData.copyWith(
                                            color: s.bar.color,
                                          ),
                                        ))
                                    .toList(),
                              ),
                            ),
                            lineBarsData: [
                              LineChartBarData(
                                spots: heapSpots,
                                color: NvrColors.accent,
                                barWidth: 2,
                                dotData: const FlDotData(show: false),
                                belowBarData: BarAreaData(
                                  show: true,
                                  color: NvrColors.accent.withOpacity(0.1),
                                ),
                              ),
                            ],
                          ),
                        ),
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    _LegendDot(color: NvrColors.accent),
                    const SizedBox(width: 6),
                    Text('Heap Alloc (MB)', style: NvrTypography.monoData),
                  ],
                ),
              ],
            ),
          ),

          const SizedBox(height: 16),

          // ── Current stats ──
          _sectionContainer(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('CURRENT STATS', style: NvrTypography.monoSection),
                const SizedBox(height: 10),
                if (_current == null)
                  Text('No data available', style: NvrTypography.body)
                else ...[
                  _kvRow(
                    'CPU USAGE',
                    '${(_current!['cpu_percent'] as num?)?.toStringAsFixed(1) ?? '--'}%',
                  ),
                  _kvRow(
                    'MEMORY USAGE',
                    '${(_current!['mem_percent'] as num?)?.toStringAsFixed(1) ?? '--'}%',
                  ),
                  _kvRow(
                    'GO HEAP',
                    '${(_current!['mem_alloc_mb'] as num?)?.toStringAsFixed(1) ?? '--'} MB',
                  ),
                  _kvRow(
                    'GOROUTINES',
                    '${_current!['goroutines'] ?? '--'}',
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _LegendDot extends StatelessWidget {
  final Color color;
  const _LegendDot({required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
      ),
    );
  }
}
