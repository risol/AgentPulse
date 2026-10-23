import 'package:flutter/material.dart';

import 'trend_chart.dart';

class DashboardOverview extends StatelessWidget {
  const DashboardOverview({
    required this.status,
    required this.summary,
    required this.starting,
    required this.apiAvailable,
    required this.backendError,
    required this.queryError,
    required this.range,
    required this.trendPoints,
    required this.loadingTrends,
    required this.trendError,
    required this.sessions,
    required this.sessionCursor,
    required this.loadingSessions,
    required this.loadingMoreSessions,
    required this.sessionError,
    required this.unassignedEvents,
    required this.unassignedCursor,
    required this.loadingUnassigned,
    required this.loadingMoreUnassigned,
    required this.unassignedError,
    required this.onRangeChanged,
    required this.onRefresh,
    required this.onRestartBackend,
    required this.onLoadMoreSessions,
    required this.onRetrySessions,
    required this.onOpenSession,
    required this.onLoadMoreUnassigned,
    required this.onRetryUnassigned,
    super.key,
  });

  final Map<String, dynamic> status;
  final Map<String, dynamic> summary;
  final bool starting;
  final bool apiAvailable;
  final String? backendError;
  final String? queryError;
  final String range;
  final List<Map<String, dynamic>> trendPoints;
  final bool loadingTrends;
  final String? trendError;
  final List<Map<String, dynamic>> sessions;
  final String? sessionCursor;
  final bool loadingSessions;
  final bool loadingMoreSessions;
  final String? sessionError;
  final List<Map<String, dynamic>> unassignedEvents;
  final String? unassignedCursor;
  final bool loadingUnassigned;
  final bool loadingMoreUnassigned;
  final String? unassignedError;
  final ValueChanged<String> onRangeChanged;
  final VoidCallback onRefresh;
  final VoidCallback onRestartBackend;
  final VoidCallback onLoadMoreSessions;
  final VoidCallback onRetrySessions;
  final ValueChanged<Map<String, dynamic>> onOpenSession;
  final VoidCallback onLoadMoreUnassigned;
  final VoidCallback onRetryUnassigned;

  String _rangeLabel(String value) => switch (value) {
    '15m' => '15 分钟',
    '1h' => '1 小时',
    '6h' => '6 小时',
    '24h' => '24 小时',
    '7d' => '7 天',
    _ => value,
  };

  @override
  Widget build(BuildContext context) {
    final signals = (status['signals'] as Map?)?.cast<String, dynamic>() ?? {};
    final counts = (summary['events'] as Map?)?.cast<String, dynamic>() ?? {};
    final isReceiving = signals.values.any(
      (value) => value is Map && value['state'] == 'receiving',
    );
    return ListView(
      padding: const EdgeInsets.fromLTRB(36, 30, 36, 36),
      children: [
        Row(
          children: [
            const Expanded(
              child: Text(
                '实时总览',
                style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700),
              ),
            ),
            Tooltip(
              message: '统计时间范围',
              child: DropdownButton<String>(
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
            ),
            IconButton(
              onPressed: onRefresh,
              tooltip: '刷新',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          '查看 Codex 遥测接收状态和最近活动',
          style: TextStyle(color: Colors.grey.shade700),
        ),
        const SizedBox(height: 24),
        if (backendError != null) _notice(backendError!, isError: true),
        if (queryError != null) _notice(queryError!, isError: true),
        if (starting) const LinearProgressIndicator(),
        _statusCard(isReceiving),
        const SizedBox(height: 18),
        Wrap(
          spacing: 16,
          runSpacing: 16,
          children: [
            _metricCard('日志', counts['logs'] ?? '—', Icons.article_outlined),
            _metricCard('指标', counts['metrics'] ?? '—', Icons.show_chart),
            _metricCard(
              'Trace',
              counts['traces'] ?? '—',
              Icons.account_tree_outlined,
            ),
          ],
        ),
        const SizedBox(height: 18),
        ..._trendCharts(),
        const SizedBox(height: 24),
        const Text(
          '信号连接情况',
          style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 12),
        for (final signal in ['logs', 'metrics', 'traces'])
          _signalRow(signal, signals[signal]),
        const SizedBox(height: 12),
        _ingestDiagnosticsCard(),
        const SizedBox(height: 20),
        const Text(
          '最近会话',
          style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 12),
        if (loadingSessions && sessions.isEmpty)
          const _LoadingCard()
        else if (sessionError != null && sessions.isEmpty)
          _RetryCard(error: sessionError!, onRetry: onRetrySessions)
        else if (sessions.isEmpty)
          const _EmptyCard('收到包含会话标识的遥测后，会在这里显示会话。')
        else
          for (final session in sessions) _sessionRow(session, onOpenSession),
        if (sessionCursor != null)
          Align(
            alignment: Alignment.center,
            child: TextButton.icon(
              onPressed: loadingMoreSessions ? null : onLoadMoreSessions,
              icon: loadingMoreSessions
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.expand_more),
              label: Text(loadingMoreSessions ? '正在加载' : '加载更多会话'),
            ),
          ),
        const SizedBox(height: 20),
        const Text(
          '未关联事件',
          style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          '这些事件没有足够的会话标识，暂不归入任何会话。',
          style: TextStyle(color: Colors.grey.shade700),
        ),
        const SizedBox(height: 12),
        if (loadingUnassigned && unassignedEvents.isEmpty)
          const _LoadingCard()
        else if (unassignedError != null && unassignedEvents.isEmpty)
          _RetryCard(error: unassignedError!, onRetry: onRetryUnassigned)
        else if (unassignedEvents.isEmpty)
          const _EmptyCard('当前没有未关联事件。')
        else
          for (final event in unassignedEvents) _unassignedEventRow(event),
        if (unassignedCursor != null)
          Align(
            alignment: Alignment.center,
            child: TextButton.icon(
              onPressed: loadingMoreUnassigned ? null : onLoadMoreUnassigned,
              icon: loadingMoreUnassigned
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.expand_more),
              label: Text(loadingMoreUnassigned ? '正在加载' : '加载更多未关联事件'),
            ),
          ),
        const SizedBox(height: 16),
        Text(
          '统计范围：最近 ${_rangeLabel(range)}。日志和 Trace 按事件计数，指标按指标点计数。',
          style: TextStyle(color: Colors.grey.shade600, fontSize: 12),
        ),
      ],
    );
  }

  Widget _statusCard(bool receiving) {
    final ready = !starting && backendError == null && apiAvailable;
    final color = receiving
        ? const Color(0xff16855b)
        : (ready ? const Color(0xffc17912) : Colors.blueGrey);
    final title = starting
        ? '正在启动本地服务'
        : (receiving ? '正在接收 Codex 数据' : (ready ? '等待遥测数据' : '服务未就绪'));
    return Card(
      elevation: 0,
      color: Colors.white,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            Icon(
              receiving ? Icons.check_circle : Icons.radar,
              color: color,
              size: 32,
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      fontSize: 17,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '本地接收地址 127.0.0.1:4318',
                    style: TextStyle(color: Colors.grey.shade700),
                  ),
                ],
              ),
            ),
            if (!ready && !starting)
              TextButton(
                onPressed: onRestartBackend,
                child: const Text('重新启动'),
              ),
          ],
        ),
      ),
    );
  }

  Widget _metricCard(String title, Object value, IconData icon) => SizedBox(
    width: 220,
    child: Card(
      elevation: 0,
      color: Colors.white,
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, color: const Color(0xff4866e8)),
            const SizedBox(height: 14),
            Text(
              '$value',
              style: const TextStyle(fontSize: 26, fontWeight: FontWeight.w700),
            ),
            Text(title, style: TextStyle(color: Colors.grey.shade700)),
          ],
        ),
      ),
    ),
  );

  List<Widget> _trendCharts() {
    final times = trendPoints
        .map((point) => (point['started_at_ns'] as num?)?.toInt() ?? 0)
        .toList();
    List<num?> values(String key) => trendPoints.map((point) {
      final value = point[key];
      return value is num ? value : null;
    }).toList();
    return [
      TrendChart(
        title: 'Token 用量趋势',
        description: '按时间桶汇总已观测到的输入和输出 Token；缺少数据的时段会留空。',
        bucketTimesNS: times,
        loading: loadingTrends,
        error: trendError,
        series: [
          TrendLineSeries(
            label: '输入 Token',
            color: const Color(0xff4866e8),
            values: values('input_tokens'),
          ),
          TrendLineSeries(
            label: '输出 Token',
            color: const Color(0xff16855b),
            values: values('output_tokens'),
          ),
        ],
      ),
      const SizedBox(height: 10),
      TrendChart(
        title: '可观测输出速率趋势',
        description: '仅使用日志与同一 trace/span 的模型请求匹配结果；没有可靠关联时不填充估算值。',
        bucketTimesNS: times,
        loading: loadingTrends,
        error: trendError,
        series: [
          TrendLineSeries(
            label: '请求级输出速率（Token/秒）',
            color: const Color(0xff16855b),
            values: values('average_request_rate_tokens_per_second'),
          ),
        ],
      ),
      const SizedBox(height: 10),
      TrendChart(
        title: 'TTFT 趋势',
        description: '仅对来源提供 TTFT 的样本取平均；没有样本的时间桶显示为空白。',
        bucketTimesNS: times,
        loading: loadingTrends,
        error: trendError,
        series: [
          TrendLineSeries(
            label: '平均 TTFT（毫秒）',
            color: const Color(0xff8a4de8),
            values: values('average_ttft_ms'),
          ),
        ],
      ),
      const SizedBox(height: 10),
      TrendChart(
        title: '工具耗时趋势',
        description: '展示每个时间桶中有耗时记录的工具调用平均耗时，不把父子调用时长相加。',
        bucketTimesNS: times,
        loading: loadingTrends,
        error: trendError,
        series: [
          TrendLineSeries(
            label: '平均工具耗时（毫秒）',
            color: const Color(0xffc17912),
            values: values('average_duration_ms'),
          ),
        ],
      ),
    ];
  }

  Widget _signalRow(String name, dynamic value) {
    final state = value is Map ? value['state'] : null;
    final active = state == 'receiving';
    final label = active ? '已连接' : (state == 'stale' ? '暂未收到新数据' : '等待数据');
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: Icon(
          active ? Icons.check_circle : Icons.schedule,
          color: active ? const Color(0xff16855b) : Colors.blueGrey,
        ),
        title: Text(
          {'logs': 'Logs', 'metrics': 'Metrics', 'traces': 'Traces'}[name]!,
        ),
        trailing: Text(
          label,
          style: TextStyle(
            color: active ? const Color(0xff16855b) : Colors.blueGrey,
          ),
        ),
      ),
    );
  }

  Widget _ingestDiagnosticsCard() {
    final diagnostics =
        (status['ingest_diagnostics'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final queue =
        (status['ingest_queue'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final recent = ((diagnostics['recent'] as List?) ?? const [])
        .whereType<Map>()
        .map((item) => item.cast<String, dynamic>())
        .take(3)
        .toList();
    const labels = <String, String>{
      'unsupported_content_type': '请求格式不受支持',
      'request_too_large': '请求超过大小限制',
      'invalid_request_body': '请求内容读取失败',
      'invalid_protobuf': '遥测数据格式无效',
      'storage_failure': '写入本地数据库失败',
    };
    final count = diagnostics['count_last_24_hours'] ?? '—';
    final depth = queue['depth'];
    final capacity = queue['capacity'];
    final rejected = queue['rejected'] ?? '—';
    return Card(
      elevation: 0,
      color: Colors.white,
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '采集诊断',
              style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 6),
            Text(
              '最近 200 条诊断中，24 小时内有 $count 条；写入队列 ${depth ?? '—'} / ${capacity ?? '—'}，拒绝 $rejected 次。',
              style: TextStyle(color: Colors.grey.shade700),
            ),
            if (recent.isNotEmpty) ...[
              const Divider(height: 22),
              for (final item in recent)
                Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Text(
                    '${item['signal'] ?? '遥测'} · ${labels[item['code']] ?? '采集异常'} · ${_diagnosticTime(item['occurred_at_ns'])}',
                    style: TextStyle(color: Colors.grey.shade800),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }

  String _diagnosticTime(dynamic value) {
    if (value is! num || value <= 0) return '时间未知';
    final time = DateTime.fromMicrosecondsSinceEpoch(value.toInt() ~/ 1000)
        .toLocal();
    final hour = time.hour.toString().padLeft(2, '0');
    final minute = time.minute.toString().padLeft(2, '0');
    return '$hour:$minute';
  }

  Widget _sessionRow(
    Map<String, dynamic> session,
    ValueChanged<Map<String, dynamic>> onOpen,
  ) {
    final id = session['id'] as String? ?? '';
    final externalId = session['external_id'] as String?;
    final displayId = externalId?.isNotEmpty == true ? externalId! : id;
    final shortId = displayId.length > 24
        ? '${displayId.substring(0, 21)}…'
        : displayId;
    final model = session['model'] as String?;
    final input = session['input_tokens'] ?? '—';
    final output = session['output_tokens'] ?? '—';
    final events = session['event_count'] ?? 0;
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: const Icon(
          Icons.chat_bubble_outline,
          color: Color(0xff4866e8),
        ),
        title: Text('会话 $shortId'),
        subtitle: Text(
          '${model ?? '模型未知'} · $events 条事件 · 输入 $input / 输出 $output tokens',
        ),
        trailing: Text(
          session['data_quality'] == 'complete' ? '时间完整' : '时间或字段缺失',
          style: const TextStyle(fontSize: 12, color: Colors.blueGrey),
        ),
        onTap: () => onOpen(session),
      ),
    );
  }

  Widget _unassignedEventRow(Map<String, dynamic> event) {
    final kind = event['kind'] as String? ?? 'telemetry';
    final signal = event['signal'] as String? ?? 'unknown';
    final turnId = event['turn_id'] as String?;
    final tool = event['tool'] as String?;
    final model = event['model'] as String?;
    final details = <String>['来源 ${event['source_id'] ?? '未知'}', signal];
    if (turnId != null && turnId.isNotEmpty) details.add('Turn $turnId');
    if (tool != null && tool.isNotEmpty) details.add('工具 $tool');
    if (model != null && model.isNotEmpty) details.add('模型 $model');
    if (event['data_quality'] == 'partial') details.add('部分数据');
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: const Icon(Icons.link_off, color: Colors.blueGrey),
        title: Text(kind),
        subtitle: Text(details.join(' · ')),
        trailing: Text(
          _formatTime(event['timestamp']),
          style: const TextStyle(fontSize: 11),
        ),
      ),
    );
  }

  String _formatTime(dynamic value) {
    if (value is! String) return '时间未知';
    final parsed = DateTime.tryParse(value)?.toLocal();
    if (parsed == null) return '时间未知';
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(parsed.hour)}:${two(parsed.minute)}:${two(parsed.second)}';
  }

  Widget _notice(String message, {bool isError = false}) => Container(
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

class _LoadingCard extends StatelessWidget {
  const _LoadingCard();

  @override
  Widget build(BuildContext context) => const Card(
    elevation: 0,
    color: Colors.white,
    child: Padding(
      padding: EdgeInsets.all(18),
      child: Center(child: CircularProgressIndicator()),
    ),
  );
}

class _RetryCard extends StatelessWidget {
  const _RetryCard({required this.error, required this.onRetry});

  final String error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Card(
    elevation: 0,
    color: Colors.white,
    child: Padding(
      padding: const EdgeInsets.all(18),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(error),
          TextButton(onPressed: onRetry, child: const Text('重试')),
        ],
      ),
    ),
  );
}

class _EmptyCard extends StatelessWidget {
  const _EmptyCard(this.message);

  final String message;

  @override
  Widget build(BuildContext context) => Card(
    elevation: 0,
    color: Colors.white,
    child: Padding(
      padding: const EdgeInsets.all(18),
      child: Text(message, style: TextStyle(color: Colors.grey.shade700)),
    ),
  );
}
