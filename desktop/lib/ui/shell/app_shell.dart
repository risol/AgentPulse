import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../components/ap_components.dart';
import '../theme/ap_colors.dart';
import '../theme/ap_typography.dart';

class AppShell extends StatelessWidget {
  const AppShell({
    super.key,
    required this.selectedIndex,
    required this.onNavigate,
    required this.child,
    required this.status,
    required this.backendVersion,
    required this.starting,
    required this.apiAvailable,
    this.backendError,
    this.sessions = const [],
    this.onRefresh,
    this.onRestart,
    this.onFocusSearch,
  });

  final int selectedIndex;
  final ValueChanged<int> onNavigate;
  final Widget child;
  final Map<String, dynamic> status;
  final String backendVersion;
  final bool starting;
  final bool apiAvailable;
  final String? backendError;
  final List<Map<String, dynamic>> sessions;
  final VoidCallback? onRefresh;
  final VoidCallback? onRestart;
  final VoidCallback? onFocusSearch;

  static const _pages = <_NavEntry>[
    _NavEntry('Live', Icons.radar, 0, 'LIVE'),
    _NavEntry('Runs', Icons.view_timeline_outlined, 1, 'ANALYZE'),
    _NavEntry('Tools', Icons.build_outlined, 2, 'ANALYZE'),
    _NavEntry('Processes', Icons.account_tree_outlined, 3, 'ANALYZE'),
    _NavEntry('Diagnostics', Icons.monitor_heart_outlined, 4, 'SYSTEM'),
    _NavEntry('Settings', Icons.settings_outlined, 5, 'SYSTEM'),
  ];

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final compact = constraints.maxWidth < 1240;
      return CallbackShortcuts(
        bindings: <ShortcutActivator, VoidCallback>{
          const SingleActivator(LogicalKeyboardKey.digit1, control: true): () =>
              onNavigate(0),
          const SingleActivator(LogicalKeyboardKey.digit2, control: true): () =>
              onNavigate(1),
          const SingleActivator(LogicalKeyboardKey.digit3, control: true): () =>
              onNavigate(2),
          const SingleActivator(LogicalKeyboardKey.digit4, control: true): () =>
              onNavigate(3),
          const SingleActivator(LogicalKeyboardKey.comma, control: true): () =>
              onNavigate(5),
          const SingleActivator(LogicalKeyboardKey.keyK, control: true): () =>
              _showPalette(context),
          const SingleActivator(LogicalKeyboardKey.keyR): () =>
              onRefresh?.call(),
        },
        child: Focus(
          autofocus: true,
          child: Scaffold(
            body: Column(
              children: [
                _TopBar(
                  receiving: _hasReceivingSignal,
                  activeRuns: _activeRunCount,
                  onPalette: () => _showPalette(context),
                ),
                Expanded(
                  child: Row(
                    children: [
                      _Sidebar(
                        compact: compact,
                        selectedIndex: selectedIndex,
                        activeRuns: _activeRunCount,
                        onNavigate: onNavigate,
                      ),
                      const VerticalDivider(width: 1, thickness: 1),
                      Expanded(child: child),
                    ],
                  ),
                ),
                _StatusBar(
                  ready: apiAvailable && backendError == null && !starting,
                  starting: starting,
                  backendError: backendError,
                  receiverAddress: status['receiver_address'] as String?,
                  signals:
                      (status['signals'] as Map?)?.cast<String, dynamic>() ??
                      const {},
                  backendVersion: backendVersion,
                  onRestart: onRestart,
                ),
              ],
            ),
          ),
        ),
      );
    },
  );

  bool get _hasReceivingSignal {
    final signals = (status['signals'] as Map?)?.values ?? const [];
    return signals.any(
      (value) => value is Map && value['state'] == 'receiving',
    );
  }

  int get _activeRunCount {
    final cutoff = DateTime.now().subtract(const Duration(minutes: 2));
    return sessions.where((session) {
      final raw = session['last_activity_at'];
      final value = raw is String ? DateTime.tryParse(raw)?.toLocal() : null;
      return value != null && value.isAfter(cutoff);
    }).length;
  }

  void _showPalette(BuildContext context) {
    showDialog<void>(
      context: context,
      builder: (context) => Dialog(
        backgroundColor: ApColors.surface1,
        child: SizedBox(
          width: 460,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Padding(
                padding: EdgeInsets.fromLTRB(16, 16, 16, 8),
                child: Text('Go to', style: ApTypography.sectionTitle),
              ),
              for (final entry in _pages)
                ListTile(
                  dense: true,
                  leading: Icon(entry.icon, size: 18),
                  title: Text(entry.label),
                  subtitle: Text(entry.section, style: ApTypography.micro),
                  onTap: () {
                    Navigator.pop(context);
                    onNavigate(entry.index);
                  },
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _NavEntry {
  const _NavEntry(this.label, this.icon, this.index, this.section);
  final String label;
  final IconData icon;
  final int index;
  final String section;
}

class _Sidebar extends StatelessWidget {
  const _Sidebar({
    required this.compact,
    required this.selectedIndex,
    required this.activeRuns,
    required this.onNavigate,
  });
  final bool compact;
  final int selectedIndex;
  final int activeRuns;
  final ValueChanged<int> onNavigate;

  @override
  Widget build(BuildContext context) {
    final width = compact ? 68.0 : 216.0;
    return Container(
      width: width,
      color: ApColors.surface0,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (!compact)
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 18, 16, 12),
              child: Row(
                children: [
                  Icon(Icons.bolt, color: ApColors.accentStrong),
                  SizedBox(width: 9),
                  Text('AgentPulse', style: ApTypography.sectionTitle),
                ],
              ),
            )
          else
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 18),
              child: Icon(Icons.bolt, color: ApColors.accentStrong),
            ),
          const Divider(height: 1),
          const SizedBox(height: 10),
          Expanded(
            child: ListView(
              padding: EdgeInsets.symmetric(horizontal: compact ? 8 : 10),
              children: [
                for (final group in ['LIVE', 'ANALYZE', 'SYSTEM']) ...[
                  if (!compact)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(9, 12, 8, 6),
                      child: Text(
                        group,
                        style: ApTypography.micro.copyWith(
                          letterSpacing: 1.1,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                  for (final entry in AppShell._pages.where(
                    (item) => item.section == group,
                  ))
                    _NavItem(
                      entry: entry,
                      compact: compact,
                      selected: entry.index == selectedIndex,
                      badge: entry.index == 0 && activeRuns > 0
                          ? '$activeRuns'
                          : null,
                      onTap: () => onNavigate(entry.index),
                    ),
                ],
              ],
            ),
          ),
          if (!compact)
            Padding(
              padding: const EdgeInsets.fromLTRB(18, 10, 18, 14),
              child: Text(
                'CODEx RUNTIME PROFILER',
                style: ApTypography.micro.copyWith(letterSpacing: .7),
              ),
            ),
        ],
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.entry,
    required this.compact,
    required this.selected,
    required this.onTap,
    this.badge,
  });
  final _NavEntry entry;
  final bool compact;
  final bool selected;
  final VoidCallback onTap;
  final String? badge;

  @override
  Widget build(BuildContext context) {
    final color = selected ? ApColors.textPrimary : ApColors.textSecondary;
    final tile = Material(
      color: selected
          ? ApColors.accent.withValues(alpha: .14)
          : Colors.transparent,
      borderRadius: BorderRadius.circular(6),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(6),
        child: Container(
          height: 38,
          margin: const EdgeInsets.symmetric(vertical: 2),
          padding: EdgeInsets.symmetric(horizontal: compact ? 0 : 10),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(6),
            border: selected
                ? Border.all(color: ApColors.accent.withValues(alpha: .25))
                : null,
          ),
          child: compact
              ? Center(
                  child: Icon(
                    entry.icon,
                    size: 18,
                    color: selected ? ApColors.accentStrong : color,
                  ),
                )
              : Row(
                  children: [
                    Icon(
                      entry.icon,
                      size: 17,
                      color: selected ? ApColors.accentStrong : color,
                    ),
                    const SizedBox(width: 11),
                    Expanded(
                      child: Text(
                        entry.label,
                        style: ApTypography.body.copyWith(
                          color: color,
                          fontWeight: selected
                              ? FontWeight.w600
                              : FontWeight.w400,
                        ),
                      ),
                    ),
                    if (badge != null)
                      ApStatusBadge(label: badge!, tone: ApTone.accent),
                  ],
                ),
        ),
      ),
    );
    return compact ? Tooltip(message: entry.label, child: tile) : tile;
  }
}

class _TopBar extends StatelessWidget {
  const _TopBar({
    required this.receiving,
    required this.activeRuns,
    required this.onPalette,
  });
  final bool receiving;
  final int activeRuns;
  final VoidCallback onPalette;

  @override
  Widget build(BuildContext context) => Container(
    height: 54,
    padding: const EdgeInsets.symmetric(horizontal: 18),
    decoration: const BoxDecoration(
      color: ApColors.surface0,
      border: Border(bottom: BorderSide(color: ApColors.borderMuted)),
    ),
    child: Row(
      children: [
        ApStatusBadge(
          label: receiving ? 'CODEX · RECEIVING' : 'CODEX · WAITING',
          tone: receiving ? ApTone.success : ApTone.warning,
          pulse: receiving,
        ),
        const SizedBox(width: 16),
        Container(width: 1, height: 20, color: ApColors.border),
        const SizedBox(width: 16),
        Icon(
          Icons.bolt,
          size: 15,
          color: activeRuns > 0 ? ApColors.accentStrong : ApColors.textMuted,
        ),
        const SizedBox(width: 6),
        Text(
          'Recent $activeRuns',
          style: ApTypography.small.copyWith(
            color: activeRuns > 0 ? ApColors.textPrimary : ApColors.textMuted,
          ),
        ),
        const Spacer(),
        OutlinedButton.icon(
          onPressed: onPalette,
          icon: const Icon(Icons.search, size: 16),
          label: const Text('Command Palette'),
          style: OutlinedButton.styleFrom(
            foregroundColor: ApColors.textSecondary,
            side: const BorderSide(color: ApColors.border),
            visualDensity: VisualDensity.compact,
          ),
        ),
        const SizedBox(width: 8),
        const Tooltip(
          message: '快捷键 Ctrl+K',
          child: Text('Ctrl K', style: ApTypography.micro),
        ),
      ],
    ),
  );
}

class _StatusBar extends StatelessWidget {
  const _StatusBar({
    required this.ready,
    required this.starting,
    required this.backendError,
    required this.receiverAddress,
    required this.signals,
    required this.backendVersion,
    this.onRestart,
  });
  final bool ready;
  final bool starting;
  final String? backendError;
  final String? receiverAddress;
  final Map<String, dynamic> signals;
  final String backendVersion;
  final VoidCallback? onRestart;

  @override
  Widget build(BuildContext context) {
    final tone = ready
        ? ApTone.success
        : starting
        ? ApTone.warning
        : ApTone.danger;
    final label = ready
        ? 'Backend healthy'
        : starting
        ? 'Backend starting'
        : 'Backend offline';
    return Container(
      height: 30,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: const BoxDecoration(
        color: ApColors.surface0,
        border: Border(top: BorderSide(color: ApColors.borderMuted)),
      ),
      child: Row(
        children: [
          ApStatusBadge(label: label, tone: tone, pulse: starting),
          if (backendError != null) ...[
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                backendError!,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: ApTypography.micro.copyWith(color: ApColors.danger),
              ),
            ),
            if (onRestart != null)
              TextButton(
                onPressed: onRestart,
                child: const Text('Retry connection'),
              ),
          ],
          if (backendError == null) const Spacer(),
          _statusText('OTLP ${receiverAddress ?? '127.0.0.1:4318'}'),
          for (final name in ['logs', 'metrics', 'traces']) ...[
            const SizedBox(width: 14),
            _statusText('${_capitalize(name)} ${_signalAge(signals[name])}'),
          ],
          const SizedBox(width: 14),
          _statusText('v$backendVersion'),
        ],
      ),
    );
  }

  Widget _statusText(String value) => Text(value, style: ApTypography.micro);
  String _capitalize(String value) =>
      '${value[0].toUpperCase()}${value.substring(1)}';
  String _signalAge(dynamic value) {
    if (value is! Map || value['last_received_at'] is! String) return '—';
    final at = DateTime.tryParse(value['last_received_at'] as String)
        ?.toLocal();
    if (at == null) return '—';
    final seconds = DateTime.now().difference(at).inSeconds.clamp(0, 999999);
    if (seconds < 60) return '${seconds}s';
    if (seconds < 3600) return '${seconds ~/ 60}m';
    return '${seconds ~/ 3600}h';
  }
}
