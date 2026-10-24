import 'package:agentpulse/ui/components/ap_components.dart';
import 'package:agentpulse/ui/components/ap_data_table.dart';
import 'package:agentpulse/ui/theme/ap_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets(
    'renders status, quality, panel, metrics and empty/error states',
    (tester) async {
      tester.view.physicalSize = const Size(1000, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(
        MaterialApp(
          theme: ApTheme.dark,
          home: Scaffold(
            body: ListView(
              children: [
                const ApStatusBadge(label: 'READY', tone: ApTone.success),
                const ApQualityBadge(quality: ApQuality.partial),
                const SizedBox(
                  width: 110,
                  child: ApStatusBadge(
                    label: 'A LONG UNAVAILABLE STATUS',
                    tone: ApTone.warning,
                  ),
                ),
                const ApPanel(title: 'PANEL', child: Text('Panel content')),
                const ApMetricTile(label: 'Calls', value: '12'),
                const SizedBox(
                  height: 160,
                  child: ApEmptyState(
                    title: 'No records',
                    message: 'Records appear after activity.',
                  ),
                ),
                const ApErrorState(message: 'Backend unavailable'),
                ApDataTable(
                  columns: const [
                    DataColumn(label: Text('Name')),
                    DataColumn(label: Text('Count')),
                  ],
                  rows: const [
                    DataRow(
                      cells: [DataCell(Text('shell')), DataCell(Text('4'))],
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('READY'), findsOneWidget);
      expect(find.text('PARTIAL'), findsOneWidget);
      expect(find.text('PANEL'), findsOneWidget);
      expect(find.text('Panel content'), findsOneWidget);
      expect(find.text('CALLS'), findsOneWidget);
      expect(find.text('No records'), findsOneWidget);
      expect(find.text('Backend unavailable'), findsOneWidget);
      expect(find.text('shell'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  test('maps observed telemetry to complete quality', () {
    expect(ApQualityBadge.fromValue('observed'), ApQuality.complete);
    expect(ApQualityBadge.fromValue('inferred'), ApQuality.inferred);
    expect(ApQualityBadge.fromValue('unknown'), ApQuality.unavailable);
  });
}
