import 'package:flutter/material.dart';

import '../../ui/theme/ap_colors.dart';

class ProcessPage extends StatefulWidget {
  const ProcessPage({
    required this.config,
    required this.status,
    required this.processes,
    required this.nextCursor,
    required this.loading,
    required this.loadingMore,
    required this.error,
    required this.onSave,
    required this.onRefresh,
    required this.onLoadMore,
    this.configurationOnly = false,
    this.privacyOnly = false,
    super.key,
  });

  final Map<String, dynamic> config;
  final Map<String, dynamic> status;
  final List<Map<String, dynamic>> processes;
  final String? nextCursor;
  final bool loading;
  final bool loadingMore;
  final String? error;
  final ValueChanged<Map<String, dynamic>> onSave;
  final VoidCallback onRefresh;
  final VoidCallback onLoadMore;
  final bool configurationOnly;
  final bool privacyOnly;

  @override
  State<ProcessPage> createState() => _ProcessPageState();
}

class _ProcessPageState extends State<ProcessPage> {
  final _rootNames = TextEditingController();
  final _categoryRules = TextEditingController();
  bool _enabled = false;
  bool _captureExecutablePaths = false;
  bool _captureCommandLines = false;
  int _sampleInterval = 500;
  int _retentionDays = 30;
  String? _validationError;

  @override
  void initState() {
    super.initState();
    _syncConfig();
  }

  @override
  void didUpdateWidget(covariant ProcessPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.config != widget.config) _syncConfig();
  }

  void _syncConfig() {
    _enabled = widget.config['enabled'] as bool? ?? false;
    _captureExecutablePaths =
        widget.config['capture_executable_paths'] as bool? ?? false;
    _captureCommandLines =
        widget.config['capture_command_lines'] as bool? ?? false;
    _sampleInterval = widget.config['sample_interval_ms'] as int? ?? 500;
    _retentionDays = widget.config['process_retention_days'] as int? ?? 30;
    final names = (widget.config['root_process_names'] as List?) ?? const [];
    _rootNames.text = names.map((value) => '$value').join(', ');
    final rules = (widget.config['category_rules'] as List?) ?? const [];
    _categoryRules.text = rules
        .map((value) {
          final rule = (value as Map).cast<String, dynamic>();
          final category = rule['category'] as String? ?? '';
          final processNames = (rule['process_names'] as List?) ?? const [];
          return '$category=${processNames.join(',')}';
        })
        .join('\n');
  }

  @override
  void dispose() {
    _rootNames.dispose();
    _categoryRules.dispose();
    super.dispose();
  }

  void _save() {
    final names = _rootNames.text
        .split(RegExp(r'[,;\n]'))
        .map((name) => name.trim())
        .where((name) => name.isNotEmpty)
        .toList();
    if (names.isEmpty) {
      setState(() => _validationError = '至少填写一个根进程名称。');
      return;
    }
    final categoryRules = <Map<String, dynamic>>[];
    for (final rawLine in _categoryRules.text.split('\n')) {
      final line = rawLine.trim();
      if (line.isEmpty) continue;
      final separator = line.indexOf('=');
      if (separator <= 0) {
        setState(() => _validationError = '分类规则格式为“分类=进程名,进程名”。');
        return;
      }
      final category = line.substring(0, separator).trim().toLowerCase();
      final processNames = line
          .substring(separator + 1)
          .split(',')
          .map((name) => name.trim())
          .where((name) => name.isNotEmpty)
          .toList();
      if (category.isEmpty || processNames.isEmpty) {
        setState(() => _validationError = '每条分类规则都需要分类名称和至少一个进程文件名。');
        return;
      }
      categoryRules.add({'category': category, 'process_names': processNames});
    }
    setState(() => _validationError = null);
    widget.onSave({
      'enabled': _enabled,
      'capture_executable_paths': _captureExecutablePaths,
      'capture_command_lines': _captureCommandLines,
      'sample_interval_ms': _sampleInterval,
      'process_retention_days': _retentionDays,
      'root_process_names': names,
      'category_rules': categoryRules,
    });
  }

  void _savePrivacy() {
    widget.onSave({
      'enabled': _enabled,
      'capture_executable_paths': _captureExecutablePaths,
      'capture_command_lines': _captureCommandLines,
      'sample_interval_ms': _sampleInterval,
      'process_retention_days': _retentionDays,
      'root_process_names': widget.config['root_process_names'] ?? const [],
      'category_rules': widget.config['category_rules'] ?? const [],
    });
  }

  @override
  Widget build(BuildContext context) {
    final isEnabled = widget.status['enabled'] == true;
    final state = widget.status['state'] as String? ?? 'disabled';
    final settingsOnly = widget.configurationOnly || widget.privacyOnly;
    final pageChildren = <Widget>[
      if (!settingsOnly)
        Row(
          children: [
            Expanded(
              child: Text(
                widget.privacyOnly
                    ? 'Privacy'
                    : (widget.configurationOnly
                          ? 'Process Monitoring'
                          : 'Processes'),
                style: const TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            IconButton(
              onPressed: widget.onRefresh,
              tooltip: '刷新',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
      if (!settingsOnly) const SizedBox(height: 8),
      if (!settingsOnly)
        Text(
          '仅采集指定 Codex 根进程及其可见后代进程的资源快照。默认关闭。',
          style: const TextStyle(color: ApColors.textMuted),
        ),
      const SizedBox(height: 20),
      if (widget.error != null) _notice(widget.error!, isError: true),
      Card(
        elevation: 0,
        color: ApColors.surface1,
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                widget.privacyOnly ? '隐私与采集范围' : 'Process monitoring settings',
                style: TextStyle(fontWeight: FontWeight.w600, fontSize: 17),
              ),
              if (!widget.privacyOnly)
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('启用进程监控'),
                  subtitle: const Text('开启后会按下方周期采集 PID、父进程、CPU 时间和工作集内存。'),
                  value: _enabled,
                  onChanged: (value) => setState(() => _enabled = value),
                ),
              if (widget.privacyOnly) ...[
                const Text(
                  '可执行路径和命令行默认不保存。命令行仍可能包含提示词或其他敏感参数，即使常见令牌已自动遮蔽。',
                  style: TextStyle(color: ApColors.textMuted, fontSize: 12),
                ),
                const SizedBox(height: 8),
              ],
              if (widget.privacyOnly) ...[
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Capture executable path'),
                  subtitle: const Text(
                    '关闭时不保存可执行文件路径；开启时所选进程树的当前用户目录会替换为 %USERPROFILE%。',
                  ),
                  value: _captureExecutablePaths,
                  onChanged: (value) =>
                      setState(() => _captureExecutablePaths = value),
                ),
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Capture command line'),
                  subtitle: const Text(
                    '开启后保存命令行参数。常见密码、令牌和当前用户目录会自动遮蔽；其他参数仍可能包含敏感内容。',
                  ),
                  value: _captureCommandLines,
                  onChanged: (value) =>
                      setState(() => _captureCommandLines = value),
                ),
                const SizedBox(height: 12),
                FilledButton.icon(
                  onPressed: _savePrivacy,
                  icon: const Icon(Icons.save_outlined),
                  label: const Text('Save privacy settings'),
                ),
              ],
              if (!widget.privacyOnly) ...[
                const SizedBox(height: 10),
                DropdownButtonFormField<int>(
                  initialValue: _sampleInterval,
                  decoration: const InputDecoration(
                    labelText: '采样周期',
                    border: OutlineInputBorder(),
                  ),
                  items: const [
                    DropdownMenuItem(value: 250, child: Text('250 毫秒')),
                    DropdownMenuItem(value: 500, child: Text('500 毫秒')),
                    DropdownMenuItem(value: 1000, child: Text('1 秒')),
                    DropdownMenuItem(value: 2000, child: Text('2 秒')),
                  ],
                  onChanged: (value) {
                    if (value != null) {
                      setState(() => _sampleInterval = value);
                    }
                  },
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<int>(
                  initialValue: _retentionDays,
                  decoration: const InputDecoration(
                    labelText: '进程历史保留期限',
                    helperText: '仅清理已退出的进程记录及采样点；运行中的进程会继续保留。',
                    border: OutlineInputBorder(),
                  ),
                  items: const [
                    DropdownMenuItem(value: 1, child: Text('1 天')),
                    DropdownMenuItem(value: 7, child: Text('7 天')),
                    DropdownMenuItem(value: 30, child: Text('30 天')),
                    DropdownMenuItem(value: 90, child: Text('90 天')),
                    DropdownMenuItem(value: 180, child: Text('180 天')),
                    DropdownMenuItem(value: 365, child: Text('365 天')),
                  ],
                  onChanged: (value) {
                    if (value != null) setState(() => _retentionDays = value);
                  },
                ),
                const SizedBox(height: 14),
                TextField(
                  controller: _rootNames,
                  decoration: const InputDecoration(
                    labelText: '根进程名称',
                    hintText: 'codex.exe',
                    helperText: '多个名称用逗号分隔；只填写文件名，不记录路径或命令行参数。',
                    border: OutlineInputBorder(),
                  ),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _categoryRules,
                  minLines: 4,
                  maxLines: 7,
                  decoration: const InputDecoration(
                    labelText: '进程分类规则',
                    hintText:
                        'shell=cmd.exe,pwsh.exe\nbuild=msbuild.exe,ninja.exe',
                    helperText: '每行一条“分类=进程文件名”规则，按文件名精确匹配；留空可关闭分类。',
                    border: OutlineInputBorder(),
                    alignLabelWithHint: true,
                  ),
                ),
                if (_validationError != null) ...[
                  const SizedBox(height: 8),
                  _notice(_validationError!, isError: true),
                ],
                const SizedBox(height: 16),
                FilledButton.icon(
                  onPressed: _save,
                  icon: const Icon(Icons.save_outlined),
                  label: const Text('保存设置'),
                ),
              ],
            ],
          ),
        ),
      ),
      if (!settingsOnly) ...[
        const SizedBox(height: 16),
        Card(
          elevation: 0,
          color: ApColors.surface1,
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        _statusLabel(state, isEnabled),
                        style: const TextStyle(
                          fontWeight: FontWeight.w600,
                          fontSize: 17,
                        ),
                      ),
                    ),
                    Text('${widget.status['active_processes'] ?? 0} 个进程'),
                  ],
                ),
                const SizedBox(height: 6),
                Text(
                  '最近采样：${_formatTimestamp(widget.status['last_sampled_at_ns'])} · 无法读取资源信息：${widget.status['incomplete_count'] ?? 0} 个',
                  style: const TextStyle(
                    color: ApColors.textMuted,
                    fontSize: 12,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  '保留 ${widget.config['process_retention_days'] ?? 30} 天 · 最近清理：${_formatTimestamp(widget.status['last_cleanup_at_ns'])} · 清理 ${widget.status['last_cleanup_rows'] ?? 0} 条',
                  style: const TextStyle(
                    color: ApColors.textMuted,
                    fontSize: 12,
                  ),
                ),
                if (widget.status['cleanup_error'] != null) ...[
                  const SizedBox(height: 6),
                  _notice('进程历史清理失败，将在后续周期重试。', isError: true),
                ],
                const SizedBox(height: 8),
                Text(
                  '轮询可能漏掉两个采样点之间启动并退出的短命进程。',
                  style: const TextStyle(
                    color: ApColors.textMuted,
                    fontSize: 12,
                  ),
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 12),
        if (widget.loading)
          const LinearProgressIndicator()
        else if (widget.processes.isEmpty)
          Card(
            elevation: 0,
            color: ApColors.surface1,
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Text(isEnabled ? '尚未发现匹配的进程。' : '启用监控并保存后，进程快照会显示在这里。'),
            ),
          )
        else ...[
          for (final process in widget.processes) _processCard(process),
          if (widget.nextCursor != null)
            Center(
              child: OutlinedButton.icon(
                onPressed: widget.loadingMore ? null : widget.onLoadMore,
                icon: widget.loadingMore
                    ? const SizedBox.square(
                        dimension: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.expand_more),
                label: Text(widget.loadingMore ? '正在加载' : '加载更多进程记录'),
              ),
            ),
        ],
      ],
    ];
    if (settingsOnly) {
      return Padding(
        padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: pageChildren,
        ),
      );
    }
    return ListView(
      padding: const EdgeInsets.fromLTRB(36, 30, 36, 36),
      children: pageChildren,
    );
  }

  Widget _processCard(Map<String, dynamic> process) {
    final ended = process['ended_at'] != null;
    final pid = process['pid'] ?? '—';
    final parent = process['parent_pid'] ?? '—';
    final name = process['image_name'] as String? ?? '未知进程';
    final category = process['command_category'] as String?;
    return Card(
      elevation: 0,
      color: ApColors.surface1,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: Icon(
          ended ? Icons.stop_circle_outlined : Icons.memory_outlined,
          color: ended ? ApColors.textMuted : ApColors.accentStrong,
        ),
        title: Text(name),
        subtitle: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '${category == null ? '未分类' : '分类：$category'} · PID $pid · 父进程 $parent · ${ended ? '已退出' : '运行中'} · ${process['sample_quality'] ?? '数据不足'}',
            ),
            if (process['executable_path'] is String &&
                (process['executable_path'] as String).isNotEmpty)
              Text(
                '路径：${process['executable_path']}',
                softWrap: true,
                style: const TextStyle(fontSize: 12),
              ),
            if (process['command_line'] is String &&
                (process['command_line'] as String).isNotEmpty)
              Text(
                '命令行：${process['command_line']}',
                softWrap: true,
                style: const TextStyle(fontSize: 12),
              ),
            Text(_associationLabel(process)),
          ],
        ),
        trailing: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Text(_formatCPU(process['cpu_time_ms'])),
            Text(_formatMemory(process['working_set_bytes'])),
          ],
        ),
      ),
    );
  }

  String _associationLabel(Map<String, dynamic> process) {
    final method = process['association_method'] as String? ?? 'unassigned';
    if (method == 'unassigned') return '工具调用：未关联';
    final name = process['associated_tool_name'] as String? ?? '工具调用';
    final score = process['association_confidence'];
    final confidence = score is num ? ' · 置信度 ${(score * 100).round()}%' : '';
    final methodLabel = switch (method) {
      'explicit_process_pid' => '直接进程标识',
      'parent_pid_time_overlap' => '父进程与时间匹配',
      'time_and_category_unique_candidate' => '类别与时间推断',
      _ => '推断关联',
    };
    return '工具调用：$name · $methodLabel$confidence';
  }

  Widget _notice(String text, {bool isError = false}) => Container(
    padding: const EdgeInsets.all(12),
    decoration: BoxDecoration(
      color: isError
          ? ApColors.danger.withValues(alpha: .1)
          : ApColors.accent.withValues(alpha: .1),
      borderRadius: BorderRadius.circular(8),
    ),
    child: Text(text),
  );

  String _statusLabel(String state, bool enabled) {
    if (!enabled) return '进程监控已关闭';
    if (state == 'error') return '采集遇到问题';
    if (state == 'sampling') return '正在采集进程快照';
    return '正在启动进程监控';
  }

  String _formatTimestamp(dynamic value) {
    if (value is! int || value <= 0) return '暂无';
    final time = DateTime.fromMicrosecondsSinceEpoch(value ~/ 1000).toLocal();
    String two(int number) => number.toString().padLeft(2, '0');
    return '${time.year}-${two(time.month)}-${two(time.day)} ${two(time.hour)}:${two(time.minute)}:${two(time.second)}';
  }

  String _formatCPU(dynamic value) {
    if (value is! num) return 'CPU 时间 —';
    return 'CPU 时间 ${value.toStringAsFixed(1)} ms';
  }

  String _formatMemory(dynamic value) {
    if (value is! num) return '工作集 —';
    return '工作集 ${(value / (1024 * 1024)).toStringAsFixed(1)} MB';
  }
}
