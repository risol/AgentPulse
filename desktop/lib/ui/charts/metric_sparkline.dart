import 'package:flutter/material.dart';

import 'metric_line_chart.dart';

class MetricSparkline extends StatelessWidget {
  const MetricSparkline({
    super.key,
    required this.values,
    required this.color,
    this.width = 84,
    this.height = 24,
  });
  final List<double> values;
  final Color color;
  final double width;
  final double height;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: width,
    child: MetricLineChart(
      values: values,
      color: color,
      height: height,
      gridColor: Colors.transparent,
    ),
  );
}
