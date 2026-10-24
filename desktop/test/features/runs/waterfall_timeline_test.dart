import 'package:agentpulse/features/sessions/waterfall_timeline.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('zooms and moves timeline selection with the keyboard', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    Map<String, dynamic>? selected;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SizedBox(
            height: 700,
            child: WaterfallTimeline(
              showDetails: false,
              records: const [
                {
                  'kind': 'model_request',
                  'start_at': '2026-10-23T10:00:00Z',
                  'end_at': '2026-10-23T10:00:01Z',
                  'duration_ms': 1000,
                  'model': 'gpt-test',
                  'span_id': 'span-1',
                },
                {
                  'kind': 'tool_call',
                  'start_at': '2026-10-23T10:00:01Z',
                  'end_at': '2026-10-23T10:00:02Z',
                  'duration_ms': 1000,
                  'tool': 'shell',
                  'span_id': 'span-2',
                },
              ],
              onSelect: (record) => selected = record,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(CustomPaint), findsWidgets);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    expect(selected?['span_id'], 'span-1');
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    expect(selected?['span_id'], 'span-2');

    await tester.tap(find.byTooltip('Zoom in'));
    await tester.pumpAndSettle();
    expect(tester.widget<Slider>(find.byType(Slider)).value, 1.25);
    expect(tester.takeException(), isNull);
  });
}
