import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';

class WaterfallTimeline extends StatefulWidget {
  const WaterfallTimeline({
    super.key,
    required this.records,
    this.showDetails = true,
    this.onSelect,
  });

  final List<Map<String, dynamic>> records;
  final bool showDetails;
  final ValueChanged<Map<String, dynamic>>? onSelect;

  @override
  State<WaterfallTimeline> createState() => _WaterfallTimelineState();
}

class _WaterfallTimelineState extends State<WaterfallTimeline> {
  final ScrollController _verticalController = ScrollController();
  final ScrollController _axisHorizontalController = ScrollController();
  final ScrollController _plotHorizontalController = ScrollController();
  double _zoom = 1;
  double? _hoverX;
  double? _hoverY;
  int? _selectedIndex;
  bool _syncingHorizontal = false;

  @override
  void initState() {
    super.initState();
    _axisHorizontalController.addListener(_syncPlotFromAxis);
    _plotHorizontalController.addListener(_syncAxisFromPlot);
  }

  @override
  void dispose() {
    _verticalController.dispose();
    _axisHorizontalController
      ..removeListener(_syncPlotFromAxis)
      ..dispose();
    _plotHorizontalController
      ..removeListener(_syncAxisFromPlot)
      ..dispose();
    super.dispose();
  }

  void _syncAxisFromPlot() =>
      _syncScroll(_plotHorizontalController, _axisHorizontalController);

  void _syncPlotFromAxis() =>
      _syncScroll(_axisHorizontalController, _plotHorizontalController);

  void _syncScroll(ScrollController source, ScrollController target) {
    if (_syncingHorizontal || !source.hasClients || !target.hasClients) return;
    _syncingHorizontal = true;
    final offset = source.position.pixels.clamp(
      0.0,
      target.position.maxScrollExtent,
    );
    if ((target.position.pixels - offset).abs() > .5) target.jumpTo(offset);
    _syncingHorizontal = false;
  }

  void _onPointerSignal(PointerSignalEvent event) {
    if (event is! PointerScrollEvent) return;
    if (HardwareKeyboard.instance.isControlPressed) {
      setState(
        () => _zoom = (_zoom + (event.scrollDelta.dy < 0 ? .25 : -.25)).clamp(
          1,
          5,
        ),
      );
    } else if (HardwareKeyboard.instance.isShiftPressed &&
        _plotHorizontalController.hasClients) {
      final position = _plotHorizontalController.position;
      final offset = (position.pixels + event.scrollDelta.dy).clamp(
        0.0,
        position.maxScrollExtent,
      );
      _plotHorizontalController.jumpTo(offset);
    }
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
        final plotWidth = math.max(480.0, constraints.maxWidth - 228) * _zoom;
        final hoverIndex = _hoverY == null
            ? null
            : (_hoverY! ~/ 34).clamp(0, records.length - 1);
        final selectedRecord = selectedIndex == null
            ? null
            : records[selectedIndex];
        final selectedX = selectedRecord == null
            ? null
            : ((_startTime(selectedRecord)!
                              .difference(windowStart)
                              .inMicroseconds /
                          1000) /
                      windowMS *
                      plotWidth)
                  .clamp(0.0, plotWidth)
                  .toDouble();
        return Focus(
          autofocus: true,
          onKeyEvent: (node, event) {
            if (event is! KeyDownEvent || records.isEmpty) {
              return KeyEventResult.ignored;
            }
            if (event.logicalKey == LogicalKeyboardKey.arrowDown ||
                event.logicalKey == LogicalKeyboardKey.arrowUp) {
              final direction = event.logicalKey == LogicalKeyboardKey.arrowDown
                  ? 1
                  : -1;
              final current = _selectedIndex ?? -1;
              final next = (current + direction).clamp(0, records.length - 1);
              setState(() => _selectedIndex = next);
              widget.onSelect?.call(records[next]);
              return KeyEventResult.handled;
            }
            if (event.logicalKey == LogicalKeyboardKey.equal ||
                event.logicalKey == LogicalKeyboardKey.add) {
              setState(() => _zoom = (_zoom + .25).clamp(1, 5));
              return KeyEventResult.handled;
            }
            if (event.logicalKey == LogicalKeyboardKey.minus) {
              setState(() => _zoom = (_zoom - .25).clamp(1, 5));
              return KeyEventResult.handled;
            }
            if (event.logicalKey == LogicalKeyboardKey.keyF) {
              setState(() => _zoom = 1);
              return KeyEventResult.handled;
            }
            return KeyEventResult.ignored;
          },
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  const Text(
                    'Timeline',
                    style: TextStyle(
                      fontSize: 12,
                      color: ApColors.textSecondary,
                    ),
                  ),
                  IconButton(
                    onPressed: () =>
                        setState(() => _zoom = (_zoom - .25).clamp(1, 5)),
                    tooltip: 'Zoom out',
                    icon: const Icon(Icons.remove, size: 16),
                  ),
                  SizedBox(
                    width: 90,
                    child: Slider(
                      value: _zoom,
                      min: 1,
                      max: 5,
                      divisions: 16,
                      label: '${_zoom.toStringAsFixed(1)}×',
                      onChanged: (value) => setState(() => _zoom = value),
                    ),
                  ),
                  IconButton(
                    onPressed: () =>
                        setState(() => _zoom = (_zoom + .25).clamp(1, 5)),
                    tooltip: 'Zoom in',
                    icon: const Icon(Icons.add, size: 16),
                  ),
                  TextButton(
                    onPressed: () => setState(() => _zoom = 1),
                    child: const Text('Fit to Run'),
                  ),
                  const Spacer(),
                  Text(
                    '${records.length} spans',
                    style: const TextStyle(
                      fontSize: 11,
                      color: ApColors.textMuted,
                    ),
                  ),
                ],
              ),
              Expanded(
                child: Column(
                  children: [
                    SizedBox(
                      height: 28,
                      child: Row(
                        children: [
                          const SizedBox(width: 220),
                          Expanded(
                            child: Listener(
                              onPointerSignal: _onPointerSignal,
                              child: SingleChildScrollView(
                                controller: _axisHorizontalController,
                                scrollDirection: Axis.horizontal,
                                child: _timeAxis(
                                  windowStart,
                                  windowEnd,
                                  plotWidth,
                                  selectedX,
                                  _hoverX,
                                ),
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                    Expanded(
                      child: Scrollbar(
                        controller: _verticalController,
                        child: SingleChildScrollView(
                          controller: _verticalController,
                          child: Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              SizedBox(
                                width: 220,
                                child: Column(
                                  children: [
                                    for (
                                      var index = 0;
                                      index < records.length;
                                      index++
                                    )
                                      _timelineLabel(
                                        records,
                                        index,
                                        selectedIndex == index,
                                      ),
                                  ],
                                ),
                              ),
                              Expanded(
                                child: Listener(
                                  onPointerSignal: _onPointerSignal,
                                  child: SingleChildScrollView(
                                    controller: _plotHorizontalController,
                                    scrollDirection: Axis.horizontal,
                                    child: MouseRegion(
                                      onHover: (event) => setState(() {
                                        _hoverX = event.localPosition.dx;
                                        _hoverY = event.localPosition.dy;
                                      }),
                                      onExit: (_) => setState(() {
                                        _hoverX = null;
                                        _hoverY = null;
                                      }),
                                      child: GestureDetector(
                                        behavior: HitTestBehavior.opaque,
                                        onTapDown: (event) {
                                          final index =
                                              (event.localPosition.dy ~/ 34)
                                                  .clamp(0, records.length - 1);
                                          setState(
                                            () => _selectedIndex = index,
                                          );
                                          widget.onSelect?.call(records[index]);
                                        },
                                        child: Tooltip(
                                          message: hoverIndex == null
                                              ? 'Select a timeline row to inspect its details.'
                                              : _tooltip(records[hoverIndex]),
                                          child: CustomPaint(
                                            size: Size(
                                              plotWidth,
                                              records.length * 34.0,
                                            ),
                                            painter: _TimelineRowsPainter(
                                              records: records,
                                              windowStart: windowStart,
                                              windowMS: windowMS,
                                              rowHeight: 34,
                                              gridColor: ApColors.borderMuted,
                                              selectedIndex: selectedIndex,
                                              hoveredIndex: hoverIndex,
                                              selectedX: selectedX,
                                              hoverX: _hoverX,
                                              kindColor: _kindColor,
                                              shortLabel: _shortKind,
                                              startTime: (record) =>
                                                  _startTime(record)!,
                                              endTime: _endTime,
                                            ),
                                          ),
                                        ),
                                      ),
                                    ),
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              if (widget.showDetails) ...[
                const SizedBox(height: 8),
                _details(selectedIndex == null ? null : records[selectedIndex]),
              ],
            ],
          ),
        );
      },
    );
  }

  Widget _timeAxis(
    DateTime start,
    DateTime end,
    double width,
    double? selectedX,
    double? hoverX,
  ) => SizedBox(
    width: width,
    height: 28,
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
                  _timeLabel(start.add(end.difference(start) * (index / 4))),
                  style: const TextStyle(
                    fontSize: 10,
                    color: ApColors.textMuted,
                  ),
                ),
                Expanded(
                  child: Container(width: 1, color: ApColors.borderMuted),
                ),
              ],
            ),
          ),
        if (selectedX != null)
          Positioned(
            left: selectedX,
            top: 0,
            bottom: 0,
            child: Container(
              width: 1,
              color: ApColors.accentStrong.withValues(alpha: .6),
            ),
          ),
        if (hoverX != null)
          Positioned(
            left: hoverX,
            top: 0,
            bottom: 0,
            child: Container(
              width: 1,
              color: ApColors.textSecondary.withValues(alpha: .45),
            ),
          ),
      ],
    ),
  );

  Widget _timelineLabel(
    List<Map<String, dynamic>> records,
    int index,
    bool selected,
  ) {
    final record = records[index];
    final level = _rowLevel(record, records);
    final color = _kindColor(record);
    return InkWell(
      onTap: () {
        setState(() => _selectedIndex = index);
        widget.onSelect?.call(record);
      },
      child: Container(
        height: 34,
        padding: EdgeInsets.only(left: 10 + level * 13, right: 8),
        decoration: BoxDecoration(
          color: selected ? ApColors.accent.withValues(alpha: .1) : null,
          border: const Border(bottom: BorderSide(color: ApColors.borderMuted)),
        ),
        child: Row(
          children: [
            Icon(_kindIcon(record), size: 13, color: color),
            const SizedBox(width: 6),
            Expanded(
              child: Text(
                _rowLabel(record),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 11,
                  color: selected
                      ? ApColors.textPrimary
                      : ApColors.textSecondary,
                  fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _details(Map<String, dynamic>? record) {
    if (record == null) {
      return const ApPanel(
        child: Padding(
          padding: EdgeInsets.all(4),
          child: Text(
            'Select a timeline record to inspect its quality and correlation.',
          ),
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
      '数据质量：${record['data_quality'] ?? '不可用'}',
      if (record['correlation_method'] is String &&
          (record['correlation_method'] as String).isNotEmpty)
        '关联：${record['correlation_method']}',
      if (record['span_id'] is String &&
          (record['span_id'] as String).isNotEmpty)
        'Span：${record['span_id']}',
    ];
    return ApPanel(
      padding: const EdgeInsets.all(12),
      child: Wrap(
        spacing: 14,
        runSpacing: 8,
        children: [
          for (final value in details)
            Text(value, style: const TextStyle(fontSize: 12)),
        ],
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

  int _rowLevel(
    Map<String, dynamic> record,
    List<Map<String, dynamic>> records,
  ) {
    final parentId = record['parent_span_id'] ?? record['parent_id'];
    if (parentId == null) return 0;
    return records.any((candidate) => candidate['span_id'] == parentId) ? 1 : 0;
  }

  IconData _kindIcon(Map<String, dynamic> record) {
    final kind = '${record['kind'] ?? ''}';
    final outcome = '${record['outcome'] ?? ''}'.toLowerCase();
    if (outcome.contains('fail') || outcome.contains('error')) {
      return Icons.error_outline;
    }
    if (kind == 'model_request' || kind == 'model_event') {
      return Icons.auto_awesome_outlined;
    }
    if (kind == 'tool_call') {
      return Icons.build_outlined;
    }
    if (kind == 'command') {
      return Icons.terminal;
    }
    return Icons.circle_outlined;
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
      return ApColors.model;
    }
    if (record['kind'] == 'tool_call') return ApColors.tool;
    if (record['kind'] == 'command') return ApColors.command;
    return ApColors.inferred;
  }

  String _timeLabel(DateTime value) {
    String two(int number) => number.toString().padLeft(2, '0');
    return '${two(value.hour)}:${two(value.minute)}:${two(value.second)}';
  }
}

class _TimelineRowsPainter extends CustomPainter {
  const _TimelineRowsPainter({
    required this.records,
    required this.windowStart,
    required this.windowMS,
    required this.rowHeight,
    required this.gridColor,
    required this.selectedIndex,
    required this.hoveredIndex,
    required this.selectedX,
    required this.hoverX,
    required this.kindColor,
    required this.shortLabel,
    required this.startTime,
    required this.endTime,
  });

  final List<Map<String, dynamic>> records;
  final DateTime windowStart;
  final double windowMS;
  final double rowHeight;
  final Color gridColor;
  final int? selectedIndex;
  final int? hoveredIndex;
  final double? selectedX;
  final double? hoverX;
  final Color Function(Map<String, dynamic>) kindColor;
  final String Function(Map<String, dynamic>) shortLabel;
  final DateTime Function(Map<String, dynamic>) startTime;
  final DateTime Function(Map<String, dynamic>) endTime;

  @override
  void paint(Canvas canvas, Size size) {
    final height = records.length * rowHeight;
    final gridPaint = Paint()
      ..color = gridColor
      ..strokeWidth = 1;
    final dividerPaint = Paint()
      ..color = gridColor.withValues(alpha: .72)
      ..strokeWidth = 1;

    if (hoveredIndex != null && hoveredIndex != selectedIndex) {
      canvas.drawRect(
        Rect.fromLTWH(0, hoveredIndex! * rowHeight, size.width, rowHeight),
        Paint()..color = ApColors.surface2.withValues(alpha: .55),
      );
    }
    if (selectedIndex != null) {
      canvas.drawRect(
        Rect.fromLTWH(0, selectedIndex! * rowHeight, size.width, rowHeight),
        Paint()..color = ApColors.accent.withValues(alpha: .1),
      );
    }

    for (var index = 0; index <= 4; index++) {
      final x = size.width * index / 4;
      canvas.drawLine(Offset(x, 0), Offset(x, height), gridPaint);
    }
    for (var index = 1; index <= records.length; index++) {
      final y = index * rowHeight;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), dividerPaint);
    }

    for (var index = 0; index < records.length; index++) {
      final record = records[index];
      final start = startTime(record);
      final end = endTime(record);
      final startMS = start.difference(windowStart).inMicroseconds / 1000;
      final durationMS = math.max(
        120.0,
        end.difference(start).inMicroseconds / 1000,
      );
      final left = (startMS / windowMS * size.width)
          .clamp(0.0, size.width - 5)
          .toDouble();
      final barWidth = math
          .max(5.0, durationMS / windowMS * size.width)
          .clamp(5.0, size.width - left)
          .toDouble();
      final top = index * rowHeight + 7;
      final rect = Rect.fromLTWH(left, top, barWidth, 20);
      final radius = RRect.fromRectAndRadius(rect, const Radius.circular(4));
      final quality = '${record['data_quality'] ?? ''}'.toLowerCase();
      final inferred =
          quality == 'inferred' ||
          '${record['correlation_method'] ?? ''}'.contains('inferred');
      final outcome = '${record['outcome'] ?? ''}'.toLowerCase();
      final failed = outcome.contains('fail') || outcome.contains('error');
      final partial = quality == 'partial';
      canvas.drawRRect(
        radius,
        Paint()
          ..color = kindColor(record).withValues(alpha: inferred ? .55 : 1),
      );
      if (failed || partial || selectedIndex == index) {
        canvas.drawRRect(
          radius,
          Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = selectedIndex == index ? 1.5 : 1
            ..color = failed
                ? ApColors.danger
                : partial
                ? ApColors.warning
                : ApColors.accentStrong,
        );
      }
      if (barWidth >= 72) {
        final text = TextPainter(
          text: TextSpan(
            text: shortLabel(record),
            style: const TextStyle(color: ApColors.textPrimary, fontSize: 10),
          ),
          maxLines: 1,
          ellipsis: '…',
          textDirection: TextDirection.ltr,
        )..layout(maxWidth: math.max(0, barWidth - 8));
        text.paint(canvas, Offset(left + 4, top + (20 - text.height) / 2));
      }
    }

    if (selectedX != null) {
      canvas.drawLine(
        Offset(selectedX!, 0),
        Offset(selectedX!, height),
        Paint()
          ..color = ApColors.accentStrong.withValues(alpha: .6)
          ..strokeWidth = 1.5,
      );
    }
    if (hoverX != null) {
      canvas.drawLine(
        Offset(hoverX!, 0),
        Offset(hoverX!, height),
        Paint()
          ..color = ApColors.textSecondary.withValues(alpha: .45)
          ..strokeWidth = 1,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _TimelineRowsPainter oldDelegate) =>
      oldDelegate.records != records ||
      oldDelegate.windowStart != windowStart ||
      oldDelegate.windowMS != windowMS ||
      oldDelegate.selectedIndex != selectedIndex ||
      oldDelegate.hoveredIndex != hoveredIndex ||
      oldDelegate.selectedX != selectedX ||
      oldDelegate.hoverX != hoverX ||
      oldDelegate.gridColor != gridColor;
}
