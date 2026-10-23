import 'dart:async';
import 'dart:convert';
import 'dart:io';

class BackendSession {
  BackendSession._({
    required this._process,
    required this.apiUri,
    required this.apiToken,
    required this.version,
  });

  final Process _process;
  final Uri apiUri;
  final String apiToken;
  final String version;

  Future<int> get exitCode => _process.exitCode;

  static Future<BackendSession> start() async {
    final executable = await _findBackend();
    final process = await Process.start(
      executable.$1,
      executable.$2,
      workingDirectory: executable.$3,
      runInShell: false,
    );
    final stderr = StringBuffer();
    final stderrComplete = Completer<void>();
    process.stderr
        .transform(utf8.decoder)
        .listen(
          (chunk) {
            const maximumErrorLength = 4096;
            final remaining = maximumErrorLength - stderr.length;
            if (remaining > 0) {
              final length = chunk.length < remaining
                  ? chunk.length
                  : remaining;
              stderr.write(chunk.substring(0, length));
            }
          },
          onDone: () {
            if (!stderrComplete.isCompleted) stderrComplete.complete();
          },
          onError: (Object _) {
            if (!stderrComplete.isCompleted) stderrComplete.complete();
          },
        );
    try {
      final readyLine = await process.stdout
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .first
          .timeout(const Duration(minutes: 10));
      final handshake = BackendHandshake.parse(readyLine);
      return BackendSession._(
        process: process,
        apiUri: handshake.apiUri,
        apiToken: handshake.apiToken,
        version: handshake.version,
      );
    } catch (error) {
      await _stopProcess(process);
      int? exitCode;
      try {
        exitCode = await process.exitCode.timeout(const Duration(seconds: 1));
      } catch (_) {}
      try {
        await stderrComplete.future.timeout(const Duration(seconds: 1));
      } catch (_) {}
      throw BackendStartupException.fromFailure(
        error,
        stderr: stderr.toString(),
        exitCode: exitCode,
      );
    }
  }

  Future<void> stop() => _stopProcess(_process);

  static Future<void> _stopProcess(Process process) async {
    try {
      await process.stdin.close();
      await process.exitCode.timeout(const Duration(seconds: 10));
    } catch (_) {
      process.kill();
    }
  }

  static Future<(String, List<String>, String)> _findBackend() async {
    final override = Platform.environment['AGENTPULSE_BACKEND'];
    if (override != null && override.isNotEmpty) {
      return (override, ['--handshake-stdout'], Directory.current.path);
    }
    final cwd = Directory.current.absolute;
    final executableDir = File(Platform.resolvedExecutable).parent;
    final candidates = <Directory>[
      executableDir,
      Directory('${cwd.path}${Platform.pathSeparator}backend'),
      Directory('${cwd.parent.path}${Platform.pathSeparator}backend'),
      Directory('${cwd.parent.path}${Platform.pathSeparator}dist'),
      Directory('${cwd.path}${Platform.pathSeparator}dist'),
    ];
    for (final directory in candidates) {
      for (final name in ['agentpulse-backend.exe', 'agentpulse.exe']) {
        final binary = File('${directory.path}${Platform.pathSeparator}$name');
        if (await binary.exists()) {
          return (binary.path, ['--handshake-stdout'], directory.path);
        }
      }
    }
    throw StateError(
      '未找到 AgentPulse 后端程序。请先运行 Windows 构建脚本，或设置 AGENTPULSE_BACKEND。',
    );
  }
}

class BackendStartupException implements Exception {
  const BackendStartupException(this.message);

  final String message;

  factory BackendStartupException.fromFailure(
    Object error, {
    required String stderr,
    int? exitCode,
  }) {
    final output = '$stderr $error'.toLowerCase();
    if (output.contains('address already in use') ||
        output.contains('only one usage of each socket address') ||
        output.contains('wsaeaddrinuse') ||
        RegExp(r'\b10048\b').hasMatch(output)) {
      return const BackendStartupException('OTLP 接收端口 4318 已被占用。请关闭现有接收器后重试。');
    }
    if (error is TimeoutException) {
      return const BackendStartupException('等待本地采集服务就绪超时，请重试。');
    }
    if (error is FormatException) {
      return const BackendStartupException('本地采集服务返回了无效的启动握手。');
    }
    final code = exitCode == null ? '未知' : '$exitCode';
    return BackendStartupException('本地采集服务未能就绪（退出码 $code）。');
  }

  @override
  String toString() => message;
}

class BackendHandshake {
  const BackendHandshake({
    required this.apiUri,
    required this.apiToken,
    required this.version,
  });

  final Uri apiUri;
  final String apiToken;
  final String version;

  factory BackendHandshake.parse(String line) {
    final decoded = jsonDecode(line);
    if (decoded is! Map) {
      throw const FormatException('后端启动握手必须是 JSON 对象');
    }
    final address = decoded['api_address'];
    final token = decoded['api_token'];
    if (address is! String ||
        address.isEmpty ||
        token is! String ||
        token.isEmpty) {
      throw const FormatException('后端启动握手缺少 API 地址或令牌');
    }
    final apiUri = Uri.parse('http://$address');
    if (apiUri.scheme != 'http' ||
        !apiUri.hasPort ||
        apiUri.port == 0 ||
        !const {'127.0.0.1', 'localhost', '::1'}.contains(apiUri.host)) {
      throw const FormatException('后端 API 必须绑定到本机回环地址和动态端口');
    }
    return BackendHandshake(
      apiUri: apiUri,
      apiToken: token,
      version: decoded['backend_version'] as String? ?? 'development',
    );
  }
}
