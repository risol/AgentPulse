import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

final class FixedWorkflowResult {
  const FixedWorkflowResult({
    required this.exitCodes,
    required this.spanIDs,
    required this.toolCallIDs,
    required this.processPids,
  });

  final List<int> exitCodes;
  final List<String> spanIDs;
  final List<String> toolCallIDs;
  final List<int> processPids;
}

final class _WorkflowCommand {
  const _WorkflowCommand({
    required this.toolName,
    required this.callID,
    required this.spanID,
    required this.processID,
    required this.startTimeNS,
    required this.endTimeNS,
    required this.exitCode,
    this.retryIndex,
  });

  final String toolName;
  final String callID;
  final List<int> spanID;
  final int processID;
  final int startTimeNS;
  final int endTimeNS;
  final int exitCode;
  final int? retryIndex;
}

final class _WorkflowSpan {
  const _WorkflowSpan({
    required this.traceID,
    required this.spanID,
    this.parentSpanID,
    required this.name,
    required this.startTimeNS,
    required this.endTimeNS,
    required this.statusCode,
    this.statusMessage = '',
    required this.attributes,
  });

  final List<int> traceID;
  final List<int> spanID;
  final List<int>? parentSpanID;
  final String name;
  final int startTimeNS;
  final int endTimeNS;
  final int statusCode;
  final String statusMessage;
  final Map<String, Object> attributes;
}

/// Runs the fixed Windows workflow and sends its observed task/process spans
/// through the same local OTLP receiver used by the app.
Future<FixedWorkflowResult> runFixedWorkflow({
  required String address,
  required String resultDirectory,
  required String sessionID,
}) async {
  final systemRoot = Platform.environment['SystemRoot'] ?? r'C:\Windows';
  final powershell =
      '$systemRoot${Platform.pathSeparator}System32${Platform.pathSeparator}'
      'WindowsPowerShell${Platform.pathSeparator}v1.0${Platform.pathSeparator}'
      'powershell.exe';
  const turnID = 'turn-1';
  final traceID = List<int>.generate(16, (index) => 0xa0 + index);
  final taskSpanID = _spanID(1);
  final taskStartNS = _nowNS();
  final testProgramPath =
      '$resultDirectory${Platform.pathSeparator}agentpulse-e2e-test.ps1';
  await File(testProgramPath).writeAsString(
    [
      r"$ErrorActionPreference = 'Stop'",
      'Start-Sleep -Milliseconds 1600',
      r'$result = 6 * 7',
      r'if ($result -ne 42) { exit 1 }',
      "Write-Output 'AgentPulse E2E test passed'",
      'exit 0',
    ].join('\n'),
  );

  final commands = <_WorkflowCommand>[
    await _runPowerShell(
      executable: powershell,
      arguments: const [
        '-NoLogo',
        '-NoProfile',
        '-NonInteractive',
        '-Command',
        "Start-Sleep -Milliseconds 1600; Write-Output 'AgentPulse E2E success'; exit 0",
      ],
      toolName: 'exec_command',
      callID: '$sessionID-success',
      spanID: _spanID(2),
    ),
    await _runPowerShell(
      executable: powershell,
      arguments: const [
        '-NoLogo',
        '-NoProfile',
        '-NonInteractive',
        '-Command',
        'Start-Sleep -Milliseconds 1600; exit 17',
      ],
      toolName: 'exec_command',
      callID: '$sessionID-failure',
      spanID: _spanID(3),
      retryIndex: 1,
    ),
    await _runPowerShell(
      executable: powershell,
      arguments: [
        '-NoLogo',
        '-NoProfile',
        '-NonInteractive',
        '-ExecutionPolicy',
        'Bypass',
        '-File',
        testProgramPath,
      ],
      toolName: 'test_program',
      callID: '$sessionID-test',
      spanID: _spanID(4),
    ),
  ];
  final taskEndNS = _nowNS();

  final spans = <_WorkflowSpan>[
    _WorkflowSpan(
      traceID: traceID,
      spanID: taskSpanID,
      name: 'agent_task',
      startTimeNS: taskStartNS,
      endTimeNS: taskEndNS,
      statusCode: 1,
      attributes: {'conversation.id': sessionID, 'turn.id': turnID},
    ),
    for (final command in commands)
      _WorkflowSpan(
        traceID: traceID,
        spanID: command.spanID,
        parentSpanID: taskSpanID,
        name: command.toolName,
        startTimeNS: command.startTimeNS,
        endTimeNS: command.endTimeNS,
        statusCode: command.exitCode == 0 ? 1 : 2,
        statusMessage: command.exitCode == 0
            ? ''
            : 'process exited with ${command.exitCode}',
        attributes: {
          'conversation.id': sessionID,
          'turn.id': turnID,
          'tool.name': command.toolName,
          'tool.call_id': command.callID,
          'tool.type': command.toolName == 'test_program' ? 'test' : 'command',
          'process.pid': command.processID,
          if (command.retryIndex != null) 'retry_index': command.retryIndex!,
        },
      ),
  ];
  final client = HttpClient();
  try {
    await _sendTrace(client, address, _traceRequest(spans));
  } finally {
    client.close(force: true);
  }

  return FixedWorkflowResult(
    exitCodes: commands.map((command) => command.exitCode).toList(),
    spanIDs: commands.map((command) => _hex(command.spanID)).toList(),
    toolCallIDs: commands.map((command) => command.callID).toList(),
    processPids: commands.map((command) => command.processID).toList(),
  );
}

Future<_WorkflowCommand> _runPowerShell({
  required String executable,
  required List<String> arguments,
  required String toolName,
  required String callID,
  required List<int> spanID,
  int? retryIndex,
}) async {
  final startTimeNS = _nowNS();
  final process = await Process.start(executable, arguments);
  final stdoutDrain = process.stdout.drain<void>();
  final stderrDrain = process.stderr.drain<void>();
  final exitCode = await process.exitCode;
  await Future.wait([stdoutDrain, stderrDrain]);
  return _WorkflowCommand(
    toolName: toolName,
    callID: callID,
    spanID: spanID,
    processID: process.pid,
    startTimeNS: startTimeNS,
    endTimeNS: _nowNS(),
    exitCode: exitCode,
    retryIndex: retryIndex,
  );
}

List<int> _traceRequest(List<_WorkflowSpan> spans) {
  final scopeSpans = <int>[];
  for (final span in spans) {
    scopeSpans.addAll(_lengthDelimitedField(2, _encodeSpan(span)));
  }
  final resourceSpans = _lengthDelimitedField(2, scopeSpans);
  return _lengthDelimitedField(1, resourceSpans);
}

List<int> _encodeSpan(_WorkflowSpan span) {
  final bytes = <int>[
    ..._lengthDelimitedField(1, span.traceID),
    ..._lengthDelimitedField(2, span.spanID),
  ];
  if (span.parentSpanID != null) {
    bytes.addAll(_lengthDelimitedField(4, span.parentSpanID!));
  }
  bytes.addAll(_lengthDelimitedField(5, utf8.encode(span.name)));
  bytes.addAll(_fixed64Field(7, span.startTimeNS));
  bytes.addAll(_fixed64Field(8, span.endTimeNS));
  for (final entry in span.attributes.entries) {
    bytes.addAll(
      _lengthDelimitedField(9, _encodeAttribute(entry.key, entry.value)),
    );
  }
  final status = <int>[..._varintField(3, span.statusCode)];
  if (span.statusMessage.isNotEmpty) {
    status.addAll(_lengthDelimitedField(2, utf8.encode(span.statusMessage)));
  }
  bytes.addAll(_lengthDelimitedField(15, status));
  return bytes;
}

List<int> _encodeAttribute(String key, Object value) {
  late final List<int> encodedValue;
  if (value is String) {
    encodedValue = _lengthDelimitedField(1, utf8.encode(value));
  } else if (value is int) {
    encodedValue = _varintField(3, value);
  } else if (value is bool) {
    encodedValue = _varintField(2, value ? 1 : 0);
  } else {
    throw ArgumentError.value(value, 'value', 'Unsupported OTLP attribute');
  }
  return [
    ..._lengthDelimitedField(1, utf8.encode(key)),
    ..._lengthDelimitedField(2, encodedValue),
  ];
}

List<int> _fixed64Field(int field, int value) {
  final bytes = ByteData(8)..setUint64(0, value, Endian.little);
  return [..._varint((field << 3) | 1), ...bytes.buffer.asUint8List()];
}

List<int> _varintField(int field, int value) => [
  ..._varint(field << 3),
  ..._varint(value),
];

Future<void> _sendTrace(
  HttpClient client,
  String address,
  List<int> body,
) async {
  final request = await client.postUrl(Uri.parse('http://$address/v1/traces'));
  request.headers.set(HttpHeaders.contentTypeHeader, 'application/x-protobuf');
  request.add(body);
  final response = await request.close();
  await response.drain<void>();
  if (response.statusCode != HttpStatus.ok) {
    throw HttpException('OTLP trace export returned ${response.statusCode}');
  }
}

List<int> _spanID(int value) {
  final bytes = ByteData(8)..setUint64(0, value, Endian.big);
  return bytes.buffer.asUint8List();
}

int _nowNS() => DateTime.now().microsecondsSinceEpoch * 1000;

String _hex(List<int> bytes) =>
    bytes.map((value) => value.toRadixString(16).padLeft(2, '0')).join();

List<int> _lengthDelimitedField(int field, List<int> bytes) => [
  ..._varint((field << 3) | 2),
  ..._varint(bytes.length),
  ...bytes,
];

List<int> _varint(int value) {
  var remaining = value;
  final bytes = <int>[];
  while (remaining >= 0x80) {
    bytes.add((remaining & 0x7f) | 0x80);
    remaining >>= 7;
  }
  bytes.add(remaining);
  return bytes;
}
