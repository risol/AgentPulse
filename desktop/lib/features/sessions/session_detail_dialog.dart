import 'package:flutter/material.dart';

import '../../core/api/local_api_client.dart';
import 'session_process_tree.dart';
import 'waterfall_timeline.dart';

class SessionDetailDialog extends StatelessWidget {
  const SessionDetailDialog({
    required this.sessionId,
    this.titleId,
    required this.data,
    super.key,
  });

  final String sessionId;
  final String? titleId;
  final Future<List<Map<String, dynamic>>> data;

  factory SessionDetailDialog.forSession({
    required String titleId,
    required String sessionId,
    required LocalApiClient apiClient,
  }) {
    final endpoint = '/api/v1/sessions/${Uri.encodeComponent(sessionId)}';
    return SessionDetailDialog(
      sessionId: sessionId,
      titleId: titleId,
      data: Future.wait([
        apiClient.get('$endpoint/timeline?limit=100'),
        apiClient.get('$endpoint/analysis'),
        apiClient.get('$endpoint/processes'),
      ]),
    );
  }

  String get _displayId {
    final value = titleId ?? sessionId;
    return value.length > 18 ? '${value.substring(0, 15)}…' : value;
  }

  @override
  Widget build(BuildContext context) {
    final viewport = MediaQuery.sizeOf(context);
    final contentWidth = (viewport.width - 96).clamp(0.0, 920.0).toDouble();
    final contentHeight = (viewport.height - 200).clamp(0.0, 620.0).toDouble();
    return AlertDialog(
      title: Text('会话详情 · $_displayId'),
      content: SizedBox(
        width: contentWidth,
        height: contentHeight,
        child: FutureBuilder<List<Map<String, dynamic>>>(
          future: data,
          builder: (context, snapshot) {
            if (snapshot.connectionState != ConnectionState.done) {
              return const Center(child: CircularProgressIndicator());
            }
            if (snapshot.hasError) {
              return Center(child: Text('读取会话分析失败：${snapshot.error}'));
            }
            final timeline = snapshot.data![0];
            final analysis = snapshot.data![1];
            final processes = snapshot.data![2];
            final items = ((timeline['items'] as List?) ?? [])
                .map((value) => (value as Map).cast<String, dynamic>())
                .toList();
            final processItems = ((processes['items'] as List?) ?? [])
                .map((value) => (value as Map).cast<String, dynamic>())
                .toList();
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _SessionAnalyticsSummary(analysis: analysis),
                const Divider(height: 12),
                Expanded(
                  child: DefaultTabController(
                    length: 2,
                    child: Column(
                      children: [
                        const TabBar(
                          tabs: [
                            Tab(text: '时间线'),
                            Tab(text: '进程树'),
                          ],
                        ),
                        Expanded(
                          child: TabBarView(
                            children: [
                              WaterfallTimeline(records: items),
                              SessionProcessTree(processes: processItems),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            );
          },
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('关闭'),
        ),
      ],
    );
  }
}

class _SessionAnalyticsSummary extends StatelessWidget {
  const _SessionAnalyticsSummary({required this.analysis});

  final Map<String, dynamic> analysis;

  String _duration(dynamic metric) {
    final value = (metric as Map?)?['value_ns'];
    if (value is! num) return '暂不可用';
    return _milliseconds(value / 1e6);
  }

  String _milliseconds(dynamic value) =>
      value is num ? '${value.toStringAsFixed(0)} ms' : '—';

  String _rate(Map<String, dynamic> metric) {
    final value = metric['tokens_per_second'];
    if (value is! num) return '暂不可用';
    final quality = metric['quality'] == 'partial' ? '（部分）' : '';
    return '${value.toStringAsFixed(1)} tokens/s$quality';
  }

  String _percent(dynamic value) =>
      value is num ? '${(value * 100).toStringAsFixed(1)}%' : '—';

  Widget _chip(String label, String value) => Chip(
    label: Text('$label · $value', style: const TextStyle(fontSize: 11)),
    visualDensity: VisualDensity.compact,
    padding: EdgeInsets.zero,
  );

  @override
  Widget build(BuildContext context) {
    final durations =
        (analysis['durations'] as Map?)?.cast<String, dynamic>() ?? {};
    final tokens = (analysis['tokens'] as Map?)?.cast<String, dynamic>() ?? {};
    final ttft = (tokens['ttft_ms'] as Map?)?.cast<String, dynamic>() ?? {};
    final requestRate =
        (tokens['request_rate'] as Map?)?.cast<String, dynamic>() ?? {};
    final generationRate =
        (tokens['generation_rate'] as Map?)?.cast<String, dynamic>() ?? {};
    final activity = analysis['activity_window_ms'];
    final sourceEventIDs = (analysis['source_event_ids'] as List?) ?? const [];
    final tools = ((analysis['tools'] as List?) ?? const [])
        .map((value) => (value as Map).cast<String, dynamic>())
        .toList();
    final toolText = tools.isEmpty
        ? '暂无工具调用'
        : tools
              .map(
                (tool) =>
                    '${tool['tool_name']}: ${tool['total_calls']} 次（失败 ${tool['failure_calls']}，失败率 ${_percent(tool['failure_rate'])}）',
              )
              .join('；');
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 6,
            runSpacing: 2,
            children: [
              _chip('观测活动跨度', _milliseconds(activity)),
              _chip('LLM 独占', _duration(durations['llm_only'])),
              _chip('命令独占', _duration(durations['command_only'])),
              _chip('并行重叠', _duration(durations['overlap'])),
              _chip('未归因', _duration(durations['unattributed'])),
            ],
          ),
          const SizedBox(height: 2),
          Text(
            'Tokens 输入 ${analysis['input_tokens'] ?? '—'} / 输出 ${analysis['output_tokens'] ?? '—'} / 推理 ${analysis['reasoning_tokens'] ?? '—'}',
          ),
          Text(
            '请求速率 ${_rate(requestRate)} · 生成阶段 ${_rate(generationRate)} · TTFT ${_milliseconds(ttft['value'])}',
          ),
          Text('工具：$toolText'),
          Text(
            '质量：${analysis['data_quality'] == 'complete' ? '完整' : '部分'} · 来源事件 ${sourceEventIDs.length}${analysis['source_event_ids_truncated'] == true ? '（仅展示前 500 个）' : ''}',
            style: TextStyle(color: Colors.grey.shade700, fontSize: 12),
          ),
          Text(
            '活动跨度按首次与最近观测时间计算；会话没有明确结束信号时，它只是可观测区间。',
            style: TextStyle(color: Colors.grey.shade600, fontSize: 11),
          ),
        ],
      ),
    );
  }
}
