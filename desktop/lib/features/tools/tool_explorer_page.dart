import 'package:flutter/material.dart';

String formatToolDuration(dynamic value) =>
    value is num ? '${value.toStringAsFixed(0)} ms' : '—';

String formatToolPercent(dynamic value) =>
    value is num ? '${(value * 100).toStringAsFixed(1)}%' : '—';

class ToolExplorerPage extends StatelessWidget {
  const ToolExplorerPage({
    required this.range,
    required this.toolFilter,
    required this.sessionFilter,
    required this.outcomeFilter,
    required this.summaries,
    required this.calls,
    required this.nextCallCursor,
    required this.loading,
    required this.loadingMore,
    required this.error,
    required this.onRangeChanged,
    required this.onToolFilterChanged,
    required this.onSessionFilterChanged,
    required this.onOutcomeChanged,
    required this.onRefresh,
    required this.onLoadMore,
    super.key,
  });

  final String range;
  final String toolFilter;
  final String sessionFilter;
  final String outcomeFilter;
  final List<Map<String, dynamic>> summaries;
  final List<Map<String, dynamic>> calls;
  final String? nextCallCursor;
  final bool loading;
  final bool loadingMore;
  final String? error;
  final ValueChanged<String> onRangeChanged;
  final ValueChanged<String> onToolFilterChanged;
  final ValueChanged<String> onSessionFilterChanged;
  final ValueChanged<String> onOutcomeChanged;
  final VoidCallback onRefresh;
  final VoidCallback onLoadMore;

  String _rangeLabel(String value) => switch (value) {
    '15m' => '15 分钟',
    '1h' => '1 小时',
    '6h' => '6 小时',
    '24h' => '24 小时',
    '7d' => '7 天',
    _ => value,
  };

  @override
  Widget build(BuildContext context) => ListView(
    padding: const EdgeInsets.fromLTRB(36, 30, 36, 36),
    children: [
      Row(
        children: [
          const Expanded(
            child: Text(
              '工具调用分析',
              style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700),
            ),
          ),
          DropdownButton<String>(
            value: range,
            items: const [
              DropdownMenuItem(value: '15m', child: Text('15 分钟')),
              DropdownMenuItem(value: '1h', child: Text('1 小时')),
              DropdownMenuItem(value: '6h', child: Text('6 小时')),
              DropdownMenuItem(value: '24h', child: Text('24 小时')),
              DropdownMenuItem(value: '7d', child: Text('7 天')),
            ],
            onChanged: (value) {
              if (value != null) onRangeChanged(value);
            },
          ),
          IconButton(
            onPressed: loading ? null : onRefresh,
            tooltip: '刷新工具分析',
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      const SizedBox(height: 8),
      Text(
        '按最近 ${_rangeLabel(range)} 汇总调用次数、结果和耗时。缺失的状态或耗时会单独标明。',
        style: TextStyle(color: Colors.grey.shade700),
      ),
      const SizedBox(height: 12),
      _ToolSessionFilter(value: sessionFilter, onApply: onSessionFilterChanged),
      const SizedBox(height: 12),
      Wrap(
        spacing: 20,
        runSpacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('工具'),
              const SizedBox(width: 8),
              DropdownButton<String>(
                value: toolFilter,
                items: [
                  const DropdownMenuItem(value: 'all', child: Text('全部工具')),
                  if (toolFilter != 'all' &&
                      !summaries.any(
                        (summary) => summary['tool_name'] == toolFilter,
                      ))
                    DropdownMenuItem(
                      value: toolFilter,
                      child: Text(toolFilter),
                    ),
                  ...summaries.map((summary) {
                    final name = summary['tool_name'] as String? ?? '';
                    return DropdownMenuItem(value: name, child: Text(name));
                  }),
                ],
                onChanged: (value) {
                  if (value != null) onToolFilterChanged(value);
                },
              ),
            ],
          ),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('结果'),
              const SizedBox(width: 8),
              DropdownButton<String>(
                value: outcomeFilter,
                items: const [
                  DropdownMenuItem(value: 'all', child: Text('全部结果')),
                  DropdownMenuItem(value: 'successful', child: Text('成功')),
                  DropdownMenuItem(value: 'failed', child: Text('失败')),
                  DropdownMenuItem(value: 'unknown', child: Text('未知')),
                ],
                onChanged: (value) {
                  if (value != null) onOutcomeChanged(value);
                },
              ),
            ],
          ),
        ],
      ),
      const SizedBox(height: 20),
      if (error != null) _ToolNotice(error!, isError: true),
      if (loading && summaries.isEmpty) const LinearProgressIndicator(),
      if (summaries.isEmpty && !loading)
        _EmptyCard(text: error == null ? '所选时间范围内没有工具调用。' : '暂时无法读取工具调用数据。')
      else
        for (final summary in summaries) _ToolSummaryRow(summary: summary),
      const SizedBox(height: 12),
      const Text(
        '单次调用',
        style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
      ),
      const SizedBox(height: 10),
      if (loading && calls.isEmpty) const LinearProgressIndicator(),
      if (calls.isEmpty && !loading)
        _EmptyCard(text: error == null ? '此筛选条件下没有调用记录。' : '暂时无法读取单次调用。')
      else
        for (final call in calls) _ToolCallRow(call: call),
      if (nextCallCursor != null)
        Align(
          alignment: Alignment.center,
          child: FilledButton.tonalIcon(
            onPressed: loadingMore ? null : onLoadMore,
            icon: loadingMore
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.expand_more),
            label: Text(loadingMore ? '正在加载' : '加载更多调用'),
          ),
        ),
      const SizedBox(height: 12),
      Text(
        '失败率只按已知成功/失败结果计算；P50/P95 只使用可用的非负耗时。',
        style: TextStyle(color: Colors.grey.shade600, fontSize: 12),
      ),
    ],
  );
}

class _ToolSessionFilter extends StatefulWidget {
  const _ToolSessionFilter({required this.value, required this.onApply});

  final String value;
  final ValueChanged<String> onApply;

  @override
  State<_ToolSessionFilter> createState() => _ToolSessionFilterState();
}

class _ToolSessionFilterState extends State<_ToolSessionFilter> {
  late final TextEditingController _controller = TextEditingController(
    text: widget.value,
  );

  @override
  void didUpdateWidget(covariant _ToolSessionFilter oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.value != widget.value && _controller.text != widget.value) {
      _controller.value = TextEditingValue(
        text: widget.value,
        selection: TextSelection.collapsed(offset: widget.value.length),
      );
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Row(
    children: [
      Expanded(
        child: TextField(
          key: const Key('tool-session-filter'),
          controller: _controller,
          maxLength: 256,
          textInputAction: TextInputAction.search,
          onSubmitted: widget.onApply,
          decoration: const InputDecoration(
            labelText: '会话 ID',
            hintText: '粘贴会话 ID（留空显示全部）',
            helperText: '按内部会话 ID 过滤工具调用',
            border: OutlineInputBorder(),
            isDense: true,
            counterText: '',
          ),
        ),
      ),
      const SizedBox(width: 10),
      FilledButton.tonal(
        onPressed: () => widget.onApply(_controller.text),
        child: const Text('筛选会话'),
      ),
      IconButton(
        tooltip: '清除会话筛选',
        onPressed: _controller.text.isEmpty
            ? null
            : () {
                _controller.clear();
                widget.onApply('');
              },
        icon: const Icon(Icons.clear),
      ),
    ],
  );
}

class _ToolCallRow extends StatelessWidget {
  const _ToolCallRow({required this.call});

  final Map<String, dynamic> call;

  String _timestamp(dynamic value) {
    if (value is! num || value <= 0) return '时间未知';
    final parsed = DateTime.fromMicrosecondsSinceEpoch(value ~/ 1000).toLocal();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${parsed.year}-${two(parsed.month)}-${two(parsed.day)} ${two(parsed.hour)}:${two(parsed.minute)}:${two(parsed.second)}';
  }

  @override
  Widget build(BuildContext context) {
    final status = call['status'] as String?;
    final retry = call['retry_index'];
    final duration = call['duration_ms'];
    final slow = duration is num && duration >= 1000;
    final statusLabel = switch (status?.toLowerCase()) {
      'success' || 'ok' || 'completed' => '成功',
      'error' || 'failed' || 'failure' || 'cancelled' => '失败',
      _ => '结果未知',
    };
    final session = call['session_id'] as String?;
    final turn = call['turn_id'] as String?;
    final externalID = call['external_id'] as String?;
    final retryLabel = retry is num ? '重试序号 $retry' : '来源未提供重试序号';
    final quality = call['data_quality'] == 'complete' ? '字段完整' : '部分字段';
    final details = <String>[
      _timestamp(call['started_at_ns']),
      duration is num ? '${duration.toStringAsFixed(0)} ms' : '耗时未知',
      if (slow) '慢调用（≥1 秒）',
      statusLabel,
      retryLabel,
      quality,
    ];
    if (session != null) details.add('会话 $session');
    if (turn != null) details.add('Turn $turn');
    if (externalID != null && externalID.isNotEmpty) {
      details.add('调用 ID $externalID');
    }
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: Icon(
          slow ? Icons.timer_outlined : Icons.build_outlined,
          color: slow ? const Color(0xffb45b12) : const Color(0xff4866e8),
        ),
        title: Text(call['tool_name'] as String? ?? '未知工具'),
        subtitle: Text(details.join(' · ')),
        trailing: Text(
          statusLabel,
          style: TextStyle(
            color: statusLabel == '失败'
                ? const Color(0xffb3261e)
                : Colors.blueGrey,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
    );
  }
}

class _ToolSummaryRow extends StatelessWidget {
  const _ToolSummaryRow({required this.summary});

  final Map<String, dynamic> summary;

  @override
  Widget build(BuildContext context) {
    final name = summary['tool_name'] as String? ?? '未知工具';
    final total = summary['total_calls'] ?? 0;
    final success = summary['success_calls'] ?? 0;
    final failed = summary['failure_calls'] ?? 0;
    final unknown = summary['unknown_outcomes'] ?? 0;
    final p50 = formatToolDuration(summary['p50_duration_ms']);
    final p95 = formatToolDuration(summary['p95_duration_ms']);
    final quality = summary['data_quality'] == 'complete' ? '完整' : '部分';
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 10),
      child: ListTile(
        leading: const Icon(Icons.build_outlined, color: Color(0xff4866e8)),
        title: Text(name),
        subtitle: Text(
          '$success 成功 · $failed 失败 · $unknown 结果未知 · P50 $p50 · P95 $p95 · $quality数据',
        ),
        trailing: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Text(
              '$total 次',
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            Text(
              '失败率 ${formatToolPercent(summary['failure_rate'])}',
              style: const TextStyle(fontSize: 11, color: Colors.blueGrey),
            ),
          ],
        ),
      ),
    );
  }
}

class _EmptyCard extends StatelessWidget {
  const _EmptyCard({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) => Card(
    elevation: 0,
    color: Colors.white,
    child: Padding(
      padding: const EdgeInsets.all(18),
      child: Text(text, style: TextStyle(color: Colors.grey.shade700)),
    ),
  );
}

class _ToolNotice extends StatelessWidget {
  const _ToolNotice(this.message, {this.isError = false});

  final String message;
  final bool isError;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: isError ? const Color(0xfffff0ed) : const Color(0xffeef2ff),
      borderRadius: BorderRadius.circular(10),
    ),
    child: Text(
      message,
      style: TextStyle(
        color: isError ? const Color(0xff9e3826) : const Color(0xff344b9a),
      ),
    ),
  );
}
