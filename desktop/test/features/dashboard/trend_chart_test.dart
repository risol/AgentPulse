import 'package:agentpulse/features/dashboard/trend_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('empty buckets are shown as no data, not as zero', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: TrendChart(
            title: 'Token 用量趋势',
            description: '按时间桶汇总',
            bucketTimesNS: [100, 200],
            loading: false,
            series: [
              TrendLineSeries(
                label: '输入 Token',
                color: Colors.blue,
                values: [null, null],
              ),
            ],
          ),
        ),
      ),
    );

    expect(find.text('此时间范围没有可绘制的数据。'), findsOneWidget);
    expect(find.byKey(const ValueKey('trend-chart-plot')), findsNothing);
  });

  testWidgets('a measured zero renders as a chart value', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: TrendChart(
            title: 'Token 用量趋势',
            description: '按时间桶汇总',
            bucketTimesNS: [100, 200],
            loading: false,
            series: [
              TrendLineSeries(
                label: '输入 Token',
                color: Colors.blue,
                values: [0, null],
              ),
            ],
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('此时间范围没有可绘制的数据。'), findsNothing);
    expect(find.byKey(const ValueKey('trend-chart-plot')), findsOneWidget);
  });
}
