import 'dart:math' as math;

import 'package:flutter/material.dart';

class WaterfallTimeline extends StatefulWidget {
  const WaterfallTimeline({super.key, required this.records});

  final List<Map<String, dynamic>> records;

  @override
  State<WaterfallTimeline> createState() => _WaterfallTimelineState();
}

class _WaterfallTimelineState extends State<WaterfallTimeline> {
  final ScrollController _verticalController = ScrollController();
  double _zoom = 1;
  int? _selectedIndex;

  @override
  void dispose() {
    _verticalController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final records = widget.records
        .where((record) => _startTime(record) != null)
        .toList();
    if (records.isEmpty) {
      return const Center(child: Text('没有带可用时间的事件或 Span。'));
    }
    final starts = records.map((record) => _startTime(record)!).toList();
    final ends = records.map((record) => _endTime(record)).toList();
    var windowStart = starts.reduce((a, b) => a.isBefore(b) ? a : b);
    var windowEnd = ends.reduce((a, b) => a.isAfter(b) ? a : b);
    if (!windowEnd.isAfter(windowStart)) {
      windowEnd = windowStart.add(const Duration(seconds: 1));
    }
    final windowMS = windowEnd.difference(windowStart).inMicroseconds / 1000;
    final selectedIndex =
        _selectedIndex != null && _selectedIndex! < records.length
        ? _selectedIndex
        : null;

    return LayoutBuilder(
      builder: (context, constraints) {
        final plotWidth = math.max(480.0, constraints.maxWidth - 148) * _zoom;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                const Text('时间线缩放', style: TextStyle(fontSize: 12)),
                Expanded(
                  child: Slider(
                    value: _zoom,
                    min: 1,
                    max: 4,
                    divisions: 6,
                    label: '${_zoom.toStringAsFixed(1)}×',
                    onChanged: (value) => setState(() => _zoom = value),
                  ),
                ),
                Text('${records.length} 条'),
              ],
            ),
            SizedBox(
              height: math.max(120.0, constraints.maxHeight - 208),
              child: Scrollbar(
                controller: _verticalController,
                child: SingleChildScrollView(
                  controller: _verticalController,
                  child: SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    child: SizedBox(
                      width: 140 + plotWidth,
                      child: Column(
                        children: [
                          _timeAxis(windowStart, windowEnd, plotWidth),
                          for (var index = 0; index < records.length; index++)
                            _timelineRow(
                              records[index],
                              index,
                              windowStart,
                              windowMS,
                              plotWidth,
                              selectedIndex == index,
                            ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 8),
            _details(selectedIndex == null ? null : records[selectedIndex]),
          ],
        );
      },
    );
  }

  Widget _timeAxis(DateTime start, DateTime end, double width) => Row(
    children: [
      const SizedBox(width: 140),
      SizedBox(
        width: width,
        height: 26,
        child: Stack(
          children: [
            for (var index = 0; index <= 4; index++)
              Positioned(
                left: width * index / 4,
                top: 0,
                bottom: 0,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _timeLabel(
                        start.add(end.difference(start) * (index / 4)),
                      ),
                      style: const TextStyle(
                        fontSize: 10,
                        color: Colors.blueGrey,
                      ),
                    ),
                    Expanded(
                      child: Container(
                        width: 1,
                        color: const Color(0xffe8eaf0),
                      ),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    ],
  );

  Widget _timelineRow(
    Map<String, dynamic> record,
    int index,
    DateTime windowStart,
    double windowMS,
    double plotWidth,
    bool selected,
  ) {
    final start = _startTime(record)!;
    final end = _endTime(record);
    final startMS = start.difference(windowStart).inMicroseconds / 1000;
    final durationMS = math.max(
      120.0,
      end.difference(start).inMicroseconds / 1000,
    );
    final left = (startMS / windowMS * plotWidth).clamp(0.0, plotWidth);
    final barWidth = math
        .max(5.0, durationMS / windowMS * plotWidth)
        .clamp(5.0, plotWidth - left);
    final color = _kindColor(record);
    return InkWell(
      onTap: () => setState(() => _selectedIndex = index),
      child: Container(
        height: 34,
        color: selected ? const Color(0xffeef2ff) : Colors.transparent,
        child: Row(
          children: [
            SizedBox(
              width: 140,
              child: Padding(
                padding: const EdgeInsets.only(right: 8),
                child: Text(
                  _rowLabel(record),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
                  ),
                ),
              ),
            ),
            SizedBox(
              width: plotWidth,
              height: 34,
              child: Stack(
                children: [
                  Positioned.fill(
                    child: CustomPaint(
                      painter: _TimelineGridPainter(
                        color: const Color(0xffedf0f5),
                      ),
                    ),
                  ),
                  Positioned(
                    left: left.toDouble(),
                    top: 7,
                    width: barWidth.toDouble(),
                    height: 20,
                    child: Tooltip(
                      message: _tooltip(record),
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 4),
                        alignment: Alignment.centerLeft,
                        decoration: BoxDecoration(
                          color: color,
                          borderRadius: BorderRadius.circular(4),
                          border: selected
                              ? Border.all(
                                  color: const Color(0xff243b93),
                                  width: 1.5,
                                )
                              : null,
                        ),
                        child: barWidth >= 72
                            ? Text(
                                _shortKind(record),
                                maxLines: 1,
                                overflow: TextOverflow.clip,
                                style: const TextStyle(
                                  color: Colors.white,
                                  fontSize: 10,
                                ),
                              )
                            : null,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _details(Map<String, dynamic>? record) {
    if (record == null) {
      return const Card(
        elevation: 0,
        child: Padding(
          padding: EdgeInsets.all(14),
          child: Text('选择一条时间线记录查看字段、数据质量和关联方式。'),
        ),
      );
    }
    final details = <String>[
      '类型：${record['kind'] ?? '事件'}',
      '开始：${_timeLabel(_startTime(record) ?? DateTime.fromMillisecondsSinceEpoch(0))}',
      if (record['end_at'] is String)
        '结束：${_timeLabel(DateTime.tryParse(record['end_at'] as String) ?? DateTime.fromMillisecondsSinceEpoch(0))}',
      if (record['duration_ms'] is num)
        '耗时：${(record['duration_ms'] as num).toStringAsFixed(1)} ms',
      if (record['model'] is String && (record['model'] as String).isNotEmpty)
        '模型：${record['model']}',
      if (record['tool'] is String && (record['tool'] as String).isNotEmpty)
        '工具：${record['tool']}',
      if (record['outcome'] is String &&
          (record['outcome'] as String).isNotEmpty)
        '结果：${record['outcome']}',
      if (record['retry_index'] is num) '重试序号：${record['retry_index']}',
      if (record['process_pid'] is num)
        'PID：${record['process_pid']} · PPID：${record['parent_process_pid'] ?? '未知'}',
      '数据质量：${record['data_quality'] == 'complete' ? '完整' : '部分'}',
      if (record['correlation_method'] is String &&
          (record['correlation_method'] as String).isNotEmpty)
        '关联：${record['correlation_method']}',
      if (record['span_id'] is String &&
          (record['span_id'] as String).isNotEmpty)
        'Span：${record['span_id']}',
    ];
    return Card(
      elevation: 0,
      color: const Color(0xfff7f8fb),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Wrap(
          spacing: 14,
          runSpacing: 8,
          children: [
            for (final value in details)
              Text(value, style: const TextStyle(fontSize: 12)),
          ],
        ),
      ),
    );
  }

  DateTime? _startTime(Map<String, dynamic> record) {
    final value = record['start_at'] ?? record['timestamp'];
    return value is String ? DateTime.tryParse(value)?.toLocal() : null;
  }

  DateTime _endTime(Map<String, dynamic> record) {
    final end = record['end_at'];
    final parsed = end is String ? DateTime.tryParse(end)?.toLocal() : null;
    if (parsed != null && !parsed.isBefore(_startTime(record)!)) return parsed;
    return _startTime(record)!.add(const Duration(milliseconds: 120));
  }

  String _rowLabel(Map<String, dynamic> record) {
    final kind = record['kind'];
    if (kind == 'model_request' || kind == 'model_event') return '模型请求';
    if (kind == 'tool_call') return '工具 · ${record['tool'] ?? '未知'}';
    if (kind == 'command') return '命令';
    return '${record['signal'] ?? '遥测'} · ${record['span_role'] ?? '事件'}';
  }

  String _shortKind(Map<String, dynamic> record) {
    if (record['kind'] == 'model_request' || record['kind'] == 'model_event') {
      return '模型';
    }
    if (record['kind'] == 'tool_call') return '${record['tool'] ?? '工具'}';
    return 'Span';
  }

  String _tooltip(Map<String, dynamic> record) {
    final duration = record['duration_ms'];
    final status = record['outcome'];
    return '${_rowLabel(record)} · ${duration is num ? '${duration.toStringAsFixed(1)} ms' : '耗时未知'}${status is String ? ' · $status' : ''}';
  }

  Color _kindColor(Map<String, dynamic> record) {
    if (record['kind'] == 'model_request' || record['kind'] == 'model_event') {
      return const Color(0xff4866e8);
    }
    if (record['kind'] == 'tool_call') return const Color(0xff16855b);
    if (record['kind'] == 'command') return const Color(0xffc17912);
    return const Color(0xff758198);
  }

  String _timeLabel(DateTime value) {
    String two(int number) => number.toString().padLeft(2, '0');
    return '${two(value.hour)}:${two(value.minute)}:${two(value.second)}';
  }
}

class _TimelineGridPainter extends CustomPainter {
  const _TimelineGridPainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    for (var index = 0; index <= 4; index++) {
      final x = size.width * index / 4;
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);
    }
  }

  @override
  bool shouldRepaint(covariant _TimelineGridPainter oldDelegate) =>
      oldDelegate.color != color;
}
