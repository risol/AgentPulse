import 'dart:math' as math;

import 'package:flutter/material.dart';

class TrendLineSeries {
  const TrendLineSeries({
    required this.label,
    required this.color,
    required this.values,
  });

  final String label;
  final Color color;
  final List<num?> values;
}

class TrendChart extends StatelessWidget {
  const TrendChart({
    super.key,
    required this.title,
    required this.description,
    required this.bucketTimesNS,
    required this.series,
    required this.loading,
    this.error,
  });

  final String title;
  final String description;
  final List<int> bucketTimesNS;
  final List<TrendLineSeries> series;
  final bool loading;
  final String? error;

  @override
  Widget build(BuildContext context) {
    final hasSamples = series.any(
      (line) => line.values.any((value) => value != null),
    );
    return Card(
      elevation: 0,
      color: Colors.white,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(18, 16, 18, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 3),
            Text(
              description,
              style: TextStyle(color: Colors.grey.shade700, fontSize: 12),
            ),
            const SizedBox(height: 10),
            Wrap(
              spacing: 16,
              runSpacing: 6,
              children: [
                for (final line in series)
                  Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(width: 10, height: 3, color: line.color),
                      const SizedBox(width: 6),
                      Text(line.label, style: const TextStyle(fontSize: 12)),
                    ],
                  ),
              ],
            ),
            const SizedBox(height: 8),
            if (loading && !hasSamples)
              const SizedBox(
                height: 150,
                child: Center(child: CircularProgressIndicator()),
              )
            else if (error != null && !hasSamples)
              SizedBox(height: 150, child: Center(child: Text(error!)))
            else if (!hasSamples)
              const SizedBox(
                height: 150,
                child: Center(child: Text('此时间范围没有可绘制的数据。')),
              )
            else
              SizedBox(
                height: 170,
                width: double.infinity,
                child: CustomPaint(
                  key: const ValueKey('trend-chart-plot'),
                  painter: _TrendChartPainter(
                    bucketTimesNS: bucketTimesNS,
                    series: series,
                    gridColor: const Color(0xffe8eaf0),
                    labelColor: const Color(0xff687080),
                  ),
                ),
              ),
            if (loading && hasSamples)
              const LinearProgressIndicator(minHeight: 2),
            if (error != null && hasSamples)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text(
                  error!,
                  style: const TextStyle(
                    color: Color(0xff9e3826),
                    fontSize: 12,
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _TrendChartPainter extends CustomPainter {
  const _TrendChartPainter({
    required this.bucketTimesNS,
    required this.series,
    required this.gridColor,
    required this.labelColor,
  });

  final List<int> bucketTimesNS;
  final List<TrendLineSeries> series;
  final Color gridColor;
  final Color labelColor;

  @override
  void paint(Canvas canvas, Size size) {
    const left = 46.0;
    const top = 8.0;
    const right = 8.0;
    const bottom = 24.0;
    final plot = Rect.fromLTRB(
      left,
      top,
      size.width - right,
      size.height - bottom,
    );
    if (plot.width <= 0 || plot.height <= 0) return;

    var maximum = 0.0;
    for (final line in series) {
      for (final value in line.values) {
        if (value != null) maximum = math.max(maximum, value.toDouble());
      }
    }
    if (maximum <= 0) maximum = 1;

    final gridPaint = Paint()
      ..color = gridColor
      ..strokeWidth = 1;
    final labelStyle = TextStyle(color: labelColor, fontSize: 10);
    for (var index = 0; index <= 4; index++) {
      final y = plot.bottom - plot.height * index / 4;
      canvas.drawLine(Offset(plot.left, y), Offset(plot.right, y), gridPaint);
      _drawLabel(
        canvas,
        (maximum * index / 4).toStringAsFixed(maximum < 10 ? 1 : 0),
        Offset(0, y - 7),
        labelStyle,
        width: left - 8,
        alignRight: true,
      );
    }

    for (final line in series) {
      final path = Path();
      var connected = false;
      var visiblePoints = 0;
      for (var index = 0; index < line.values.length; index++) {
        final value = line.values[index];
        if (value == null) {
          connected = false;
          continue;
        }
        final x =
            plot.left +
            (line.values.length <= 1 ? 0 : index / (line.values.length - 1)) *
                plot.width;
        final y = plot.bottom - value.toDouble() / maximum * plot.height;
        final point = Offset(x, y);
        if (connected) {
          path.lineTo(point.dx, point.dy);
        } else {
          path.moveTo(point.dx, point.dy);
        }
        connected = true;
        visiblePoints++;
      }
      canvas.drawPath(
        path,
        Paint()
          ..color = line.color
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2
          ..strokeCap = StrokeCap.round
          ..strokeJoin = StrokeJoin.round,
      );
      if (visiblePoints == 1) {
        final index = line.values.indexWhere((value) => value != null);
        final value = line.values[index]!.toDouble();
        final x =
            plot.left +
            (line.values.length <= 1 ? 0 : index / (line.values.length - 1)) *
                plot.width;
        final y = plot.bottom - value / maximum * plot.height;
        canvas.drawCircle(Offset(x, y), 3, Paint()..color = line.color);
      }
    }

    if (bucketTimesNS.isNotEmpty) {
      _drawLabel(
        canvas,
        _formatAxisTime(bucketTimesNS.first),
        Offset(plot.left, plot.bottom + 5),
        labelStyle,
      );
      _drawLabel(
        canvas,
        _formatAxisTime(bucketTimesNS.last),
        Offset(plot.right - 80, plot.bottom + 5),
        labelStyle,
        width: 80,
        alignRight: true,
      );
    }
  }

  void _drawLabel(
    Canvas canvas,
    String value,
    Offset offset,
    TextStyle style, {
    double width = 100,
    bool alignRight = false,
  }) {
    final painter = TextPainter(
      text: TextSpan(text: value, style: style),
      textDirection: TextDirection.ltr,
      textAlign: alignRight ? TextAlign.right : TextAlign.left,
      maxLines: 1,
    )..layout(maxWidth: width);
    painter.paint(canvas, offset);
  }

  String _formatAxisTime(int timestampNS) {
    final time = DateTime.fromMicrosecondsSinceEpoch(timestampNS ~/ 1000)
        .toLocal();
    String two(int value) => value.toString().padLeft(2, '0');
    return '${two(time.month)}/${two(time.day)} ${two(time.hour)}:${two(time.minute)}';
  }

  @override
  bool shouldRepaint(covariant _TrendChartPainter oldDelegate) =>
      oldDelegate.bucketTimesNS != bucketTimesNS ||
      oldDelegate.series != series;
}
