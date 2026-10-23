import 'package:flutter/material.dart';

import '../../core/api/local_api_client.dart';

class SessionExplorerPage extends StatefulWidget {
  const SessionExplorerPage({
    required this.apiClient,
    required this.onOpenSession,
    super.key,
  });

  final LocalApiClient? apiClient;
  final ValueChanged<Map<String, dynamic>> onOpenSession;

  @override
  State<SessionExplorerPage> createState() => _SessionExplorerPageState();
}

class _SessionExplorerPageState extends State<SessionExplorerPage> {
  final _modelController = TextEditingController();
  List<Map<String, dynamic>> _sessions = [];
  String _range = 'all';
  String? _cursor;
  String? _error;
  int _generation = 0;
  bool _loading = false;
  bool _loadingMore = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant SessionExplorerPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.apiClient, widget.apiClient)) _load();
  }

  @override
  void dispose() {
    _generation++;
    _modelController.dispose();
    super.dispose();
  }

  Future<void> _load({bool append = false}) async {
    final api = widget.apiClient;
    if (api == null) {
      setState(() {
        _sessions = [];
        _cursor = null;
        _error = '本地服务尚未就绪。';
        _loading = false;
        _loadingMore = false;
      });
      return;
    }
    final generation = ++_generation;
    final cursor = append ? _cursor : null;
    setState(() {
      if (append) {
        _loadingMore = true;
      } else {
        _loading = true;
        _sessions = [];
        _cursor = null;
      }
      _error = null;
    });

    final query = <String, String>{'range': _range, 'limit': '25'};
    final model = _modelController.text.trim();
    if (model.isNotEmpty) query['model'] = model;
    if (cursor != null) query['cursor'] = cursor;
    try {
      final response = await api.get(
        '/api/v1/sessions?${Uri(queryParameters: query).query}',
      );
      if (!mounted || generation != _generation) return;
      final items = ((response['items'] as List?) ?? const [])
          .map((value) => (value as Map).cast<String, dynamic>())
          .toList();
      setState(() {
        _sessions = append ? [..._sessions, ...items] : items;
        _cursor = response['next_cursor'] as String?;
      });
    } catch (error) {
      if (!mounted || generation != _generation) return;
      setState(() => _error = '读取会话失败：$error');
    } finally {
      if (mounted && generation == _generation) {
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _selectRange(String? value) {
    if (value == null || value == _range) return;
    setState(() => _range = value);
    _load();
  }

  String _displayId(Map<String, dynamic> session) {
    final externalId = session['external_id'] as String?;
    final id = externalId?.isNotEmpty == true
        ? externalId!
        : session['id'] as String? ?? '';
    return id.length > 36 ? '${id.substring(0, 33)}…' : id;
  }

  String _duration(dynamic value) {
    if (value is! num || value < 0) return '未知';
    if (value < 1000) return '${value.round()} 毫秒';
    final seconds = value / 1000;
    if (seconds < 60) return '${seconds.toStringAsFixed(1)} 秒';
    final minutes = seconds / 60;
    if (minutes < 60) return '${minutes.toStringAsFixed(1)} 分钟';
    return '${(minutes / 60).toStringAsFixed(1)} 小时';
  }

  String _time(dynamic value) {
    if (value is! String) return '时间未知';
    final parsed = DateTime.tryParse(value)?.toLocal();
    if (parsed == null) return '时间未知';
    String two(int number) => number.toString().padLeft(2, '0');
    return '${parsed.year}-${two(parsed.month)}-${two(parsed.day)} ${two(parsed.hour)}:${two(parsed.minute)}';
  }

  Widget _sessionCard(Map<String, dynamic> session) {
    final model = session['model'] as String? ?? '模型未知';
    final quality = session['data_quality'] == 'complete' ? '字段较完整' : '部分字段';
    final input = session['input_tokens'] ?? '—';
    final output = session['output_tokens'] ?? '—';
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 10),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 8),
        leading: const Icon(
          Icons.chat_bubble_outline,
          color: Color(0xff4866e8),
        ),
        title: Text('会话 ${_displayId(session)}'),
        subtitle: Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Text(
            '$model · 最近活动 ${_time(session['last_activity_at'])}\n'
            '可观测跨度 ${_duration(session['activity_duration_ms'])} · ${session['event_count'] ?? 0} 条事件 · '
            '工具 ${session['tool_call_count'] ?? 0} · 命令 ${session['command_count'] ?? 0} · 失败 ${session['failed_call_count'] ?? 0}\n'
            '输入 $input / 输出 $output tokens · $quality',
          ),
        ),
        isThreeLine: true,
        trailing: const Icon(Icons.chevron_right),
        onTap: () => widget.onOpenSession(session),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => RefreshIndicator(
    onRefresh: _load,
    child: ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(36, 30, 36, 36),
      children: [
        const Text(
          '会话',
          style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 8),
        Text(
          '按活动时间和模型筛选。会话没有明确结束信号时，展示的是可观测活动跨度。',
          style: TextStyle(color: Colors.grey.shade700),
        ),
        const SizedBox(height: 16),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            SizedBox(
              width: 220,
              child: DropdownButtonFormField<String>(
                initialValue: _range,
                isExpanded: true,
                decoration: const InputDecoration(
                  labelText: '活动时间',
                  border: OutlineInputBorder(),
                  isDense: true,
                ),
                items: const [
                  DropdownMenuItem(value: 'all', child: Text('全部时间')),
                  DropdownMenuItem(value: '24h', child: Text('最近 24 小时')),
                  DropdownMenuItem(value: '7d', child: Text('最近 7 天')),
                ],
                onChanged: _selectRange,
              ),
            ),
            SizedBox(
              width: 280,
              child: TextField(
                controller: _modelController,
                textInputAction: TextInputAction.search,
                onSubmitted: (_) => _load(),
                decoration: const InputDecoration(
                  labelText: '模型包含',
                  hintText: '例如 gpt-5',
                  border: OutlineInputBorder(),
                  isDense: true,
                ),
              ),
            ),
            FilledButton.icon(
              onPressed: _loading ? null : _load,
              icon: const Icon(Icons.search),
              label: const Text('筛选'),
            ),
            TextButton(
              onPressed: () {
                setState(() {
                  _range = 'all';
                  _modelController.clear();
                });
                _load();
              },
              child: const Text('清除'),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          '当前遥测不提供可靠的会话状态或项目路径，因此不显示这两项筛选。',
          style: TextStyle(fontSize: 12, color: Colors.grey.shade600),
        ),
        const SizedBox(height: 18),
        if (_error != null)
          Card(
            color: const Color(0xfffff0ed),
            child: ListTile(
              title: Text(_error!),
              trailing: IconButton(
                onPressed: _loading ? null : _load,
                icon: const Icon(Icons.refresh),
                tooltip: '重试',
              ),
            ),
          ),
        if (_loading && _sessions.isEmpty) const LinearProgressIndicator(),
        if (!_loading && _error == null && _sessions.isEmpty)
          const Card(
            color: Colors.white,
            child: Padding(
              padding: EdgeInsets.all(20),
              child: Text('所选条件下没有会话。'),
            ),
          ),
        for (final session in _sessions) _sessionCard(session),
        if (_cursor != null)
          Center(
            child: OutlinedButton.icon(
              onPressed: _loadingMore ? null : () => _load(append: true),
              icon: _loadingMore
                  ? const SizedBox.square(
                      dimension: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.expand_more),
              label: Text(_loadingMore ? '正在加载' : '加载更多会话'),
            ),
          ),
      ],
    ),
  );
}
