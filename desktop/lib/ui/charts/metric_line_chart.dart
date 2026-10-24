import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/ap_colors.dart';

class MetricLineChart extends StatelessWidget {
  const MetricLineChart({
    super.key,
    required this.values,
    required this.color,
    this.height = 150,
    this.gridColor = ApColors.borderMuted,
  });
  final List<double> values;
  final Color color;
  final double height;
  final Color gridColor;
  @override
  Widget build(BuildContext context) => SizedBox(
    height: height,
    width: double.infinity,
    child: CustomPaint(painter: _MetricLinePainter(values, color, gridColor)),
  );
}

class _MetricLinePainter extends CustomPainter {
  const _MetricLinePainter(this.values, this.color, this.gridColor);
  final List<double> values;
  final Color color;
  final Color gridColor;
  @override
  void paint(Canvas canvas, Size size) {
    final grid = Paint()
      ..color = gridColor
      ..strokeWidth = 1;
    for (var i = 0; i < 4; i++) {
      final y = size.height * i / 3;
      canvas.drawLine(Offset.zero.translate(0, y), Offset(size.width, y), grid);
    }
    if (values.length < 2) return;
    final minValue = values.reduce(math.min);
    final maxValue = values.reduce(math.max);
    final range = math.max(1e-9, maxValue - minValue);
    final path = Path();
    for (var i = 0; i < values.length; i++) {
      final point = Offset(
        size.width * i / (values.length - 1),
        size.height - ((values[i] - minValue) / range * (size.height - 12)) - 6,
      );
      if (i == 0) {
        path.moveTo(point.dx, point.dy);
      } else {
        path.lineTo(point.dx, point.dy);
      }
    }
    final fill = Path.from(path)
      ..lineTo(size.width, size.height)
      ..lineTo(0, size.height)
      ..close();
    canvas.drawPath(
      fill,
      Paint()
        ..shader = LinearGradient(
          colors: [color.withValues(alpha: .2), color.withValues(alpha: .01)],
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
        ).createShader(Offset.zero & size),
    );
    canvas.drawPath(
      path,
      Paint()
        ..color = color
        ..strokeWidth = 2
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round,
    );
  }

  @override
  bool shouldRepaint(covariant _MetricLinePainter old) =>
      old.values != values || old.color != color || old.gridColor != gridColor;
}
