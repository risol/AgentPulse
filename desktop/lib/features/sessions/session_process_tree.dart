import 'package:flutter/material.dart';

class SessionProcessTree extends StatelessWidget {
  const SessionProcessTree({required this.processes, super.key});

  final List<Map<String, dynamic>> processes;

  @override
  Widget build(BuildContext context) {
    if (processes.isEmpty) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.account_tree_outlined, size: 36),
              SizedBox(height: 12),
              Text('此会话没有可关联的进程记录。'),
              SizedBox(height: 6),
              Text('开启进程监控后，新采集的命令进程会显示在这里。', textAlign: TextAlign.center),
            ],
          ),
        ),
      );
    }

    final byPid = <int, Map<String, dynamic>>{};
    for (final process in processes) {
      final pid = process['pid'];
      if (pid is num) byPid.putIfAbsent(pid.toInt(), () => process);
    }
    return ListView.separated(
      padding: const EdgeInsets.symmetric(vertical: 12),
      itemCount: processes.length,
      separatorBuilder: (context, index) => const SizedBox(height: 6),
      itemBuilder: (context, index) {
        final process = processes[index];
        final pid = _integer(process['pid']);
        final parentPid = _integer(process['parent_pid']);
        final depth = _depthFor(parentPid, byPid, pid);
        return Padding(
          padding: EdgeInsets.only(left: 8 + (depth * 22.0), right: 8),
          child: Card(
            margin: EdgeInsets.zero,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Icon(
                        depth == 0
                            ? Icons.computer
                            : Icons.subdirectory_arrow_right,
                        size: 18,
                        color: Theme.of(context).colorScheme.primary,
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          (process['image_name'] as String?) ?? '未知进程',
                          style: Theme.of(context).textTheme.titleSmall,
                        ),
                      ),
                      _associationChip(context, process),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Text('PID $pid · 父 PID $parentPid${_exitCode(process)}'),
                  if (process['executable_path'] is String &&
                      (process['executable_path'] as String).isNotEmpty)
                    Text('路径：${process['executable_path']}'),
                  if (process['command_line'] is String &&
                      (process['command_line'] as String).isNotEmpty)
                    Text('命令行：${process['command_line']}'),
                  Text(
                    '活动时间 ${_time(process['started_at'])} – ${_time(process['ended_at'] ?? process['last_sample_at'])}',
                  ),
                  Text(
                    'CPU ${_cpu(process['cpu_time_ms'])} · 内存 ${_memory(process['working_set_bytes'])} · ${_quality(process['sample_quality'])}',
                  ),
                  if (process['associated_tool_name'] is String)
                    Text('关联工具：${process['associated_tool_name']}'),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  Widget _associationChip(BuildContext context, Map<String, dynamic> process) {
    final method = process['association_method'] as String? ?? 'unassigned';
    final confidence = process['association_confidence'];
    final label = switch (method) {
      'explicit_process_pid' => 'PID 明确关联',
      'parent_pid_time_overlap' => '父进程与时间匹配',
      'time_and_category_unique_candidate' => '时间与类型推测',
      _ => '未关联',
    };
    final suffix = confidence is num ? ' ${((confidence * 100).round())}%' : '';
    return Chip(
      label: Text('$label$suffix', style: const TextStyle(fontSize: 11)),
      visualDensity: VisualDensity.compact,
      padding: EdgeInsets.zero,
      side: BorderSide(color: Theme.of(context).colorScheme.outlineVariant),
    );
  }

  int _depthFor(
    int? parentPid,
    Map<int, Map<String, dynamic>> byPid,
    int? pid,
  ) {
    var current = parentPid;
    var depth = 0;
    final seen = <int>{?pid};
    while (current != null &&
        byPid.containsKey(current) &&
        !seen.contains(current)) {
      seen.add(current);
      depth++;
      current = _integer(byPid[current]!['parent_pid']);
    }
    return depth;
  }

  int? _integer(dynamic value) => value is num ? value.toInt() : null;

  String _time(dynamic value) {
    if (value is! String || value.isEmpty) return '—';
    final parsed = DateTime.tryParse(value)?.toLocal();
    if (parsed == null) return value;
    final hour = parsed.hour.toString().padLeft(2, '0');
    final minute = parsed.minute.toString().padLeft(2, '0');
    final second = parsed.second.toString().padLeft(2, '0');
    return '$hour:$minute:$second';
  }

  String _cpu(dynamic value) =>
      value is num ? '${value.toStringAsFixed(0)} ms' : '—';

  String _memory(dynamic value) {
    if (value is! num) return '—';
    final megabytes = value / (1024 * 1024);
    return '${megabytes.toStringAsFixed(1)} MB';
  }

  String _quality(dynamic value) => switch (value) {
    'sampled' => '已采样',
    'exit_observed' => '已观察到退出',
    'partial' => '部分采样',
    'unknown' || null => '采样质量未知',
    _ => value.toString(),
  };

  String _exitCode(Map<String, dynamic> process) =>
      process['exit_code'] is num ? ' · 退出码 ${process['exit_code']}' : '';
}
