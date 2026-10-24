import 'package:flutter/material.dart';

import '../theme/ap_colors.dart';
import '../theme/ap_spacing.dart';
import '../theme/ap_typography.dart';

enum ApTone { neutral, success, warning, danger, info, accent }

enum ApQuality { exact, complete, partial, inferred, estimated, unavailable }

Color apToneColor(ApTone tone) => switch (tone) {
  ApTone.success => ApColors.success,
  ApTone.warning => ApColors.warning,
  ApTone.danger => ApColors.danger,
  ApTone.info => ApColors.info,
  ApTone.accent => ApColors.accentStrong,
  ApTone.neutral => ApColors.textSecondary,
};

class ApPanel extends StatelessWidget {
  const ApPanel({
    super.key,
    this.title,
    this.trailing,
    required this.child,
    this.padding = const EdgeInsets.all(ApSpacing.md),
    this.headerPadding,
    this.expandChild = false,
  });
  final String? title;
  final Widget? trailing;
  final Widget child;
  final EdgeInsetsGeometry padding;
  final EdgeInsetsGeometry? headerPadding;
  final bool expandChild;

  @override
  Widget build(BuildContext context) => Container(
    decoration: BoxDecoration(
      color: ApColors.surface1,
      borderRadius: BorderRadius.circular(ApSpacing.panelRadius),
      border: Border.all(color: ApColors.borderMuted),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (title != null)
          Padding(
            padding: headerPadding ?? const EdgeInsets.fromLTRB(16, 12, 12, 10),
            child: Row(
              children: [
                Expanded(child: Text(title!, style: ApTypography.sectionTitle)),
                ?trailing,
              ],
            ),
          ),
        if (expandChild)
          Expanded(
            child: Padding(padding: padding, child: child),
          )
        else
          Padding(padding: padding, child: child),
      ],
    ),
  );
}

class ApStatusBadge extends StatelessWidget {
  const ApStatusBadge({
    super.key,
    required this.label,
    this.tone = ApTone.neutral,
    this.pulse = false,
  });
  final String label;
  final ApTone tone;
  final bool pulse;

  @override
  Widget build(BuildContext context) {
    final color = apToneColor(tone);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .11),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: .34)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (pulse)
            _PulseDot(color: color)
          else
            Icon(_toneIcon(tone), size: 10, color: color),
          const SizedBox(width: 6),
          Flexible(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: ApTypography.micro.copyWith(
                color: color,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }

  IconData _toneIcon(ApTone value) => switch (value) {
    ApTone.success => Icons.check_circle,
    ApTone.warning => Icons.warning_amber_rounded,
    ApTone.danger => Icons.error,
    ApTone.info => Icons.info,
    ApTone.accent => Icons.bolt,
    ApTone.neutral => Icons.help_outline,
  };
}

class ApQualityBadge extends StatelessWidget {
  const ApQualityBadge({super.key, required this.quality});
  final ApQuality quality;

  @override
  Widget build(BuildContext context) {
    final (label, tone, help) = switch (quality) {
      ApQuality.exact => ('EXACT', ApTone.success, '由明确标识直接关联。'),
      ApQuality.complete => ('COMPLETE', ApTone.success, '当前指标的来源字段完整。'),
      ApQuality.partial => ('PARTIAL', ApTone.warning, '部分字段缺失或关联不完整。'),
      ApQuality.inferred => ('INFERRED', ApTone.neutral, '通过时间或父子关系推断关联。'),
      ApQuality.estimated => ('ESTIMATED', ApTone.info, '根据可用观测值估算。'),
      ApQuality.unavailable => (
        'UNAVAILABLE',
        ApTone.neutral,
        '当前没有足够数据计算此指标。',
      ),
    };
    return Tooltip(
      message: help,
      child: ApStatusBadge(label: label, tone: tone),
    );
  }

  static ApQuality fromValue(Object? value) => switch ('$value'.toLowerCase()) {
    'exact' => ApQuality.exact,
    'observed' || 'complete' => ApQuality.complete,
    'partial' => ApQuality.partial,
    'inferred' => ApQuality.inferred,
    'estimated' => ApQuality.estimated,
    _ => ApQuality.unavailable,
  };
}

class _PulseDot extends StatefulWidget {
  const _PulseDot({required this.color});
  final Color color;
  @override
  State<_PulseDot> createState() => _PulseDotState();
}

class _PulseDotState extends State<_PulseDot>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1000),
  )..repeat(reverse: true);
  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _controller,
    builder: (context, child) => Opacity(
      opacity: .55 + _controller.value * .45,
      child: Transform.scale(
        scale: .85 + _controller.value * .15,
        child: child,
      ),
    ),
    child: Icon(Icons.circle, size: 9, color: widget.color),
  );
}

class ApMetricTile extends StatelessWidget {
  const ApMetricTile({
    super.key,
    required this.label,
    required this.value,
    this.unit,
    this.quality,
    this.icon,
    this.tint,
  });
  final String label;
  final String value;
  final String? unit;
  final ApQuality? quality;
  final IconData? icon;
  final Color? tint;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final compact =
          constraints.maxHeight.isFinite && constraints.maxHeight < 130;
      final verticalPadding = compact ? 8.0 : 12.0;
      final sectionGap = compact ? 3.0 : 7.0;
      return Container(
        constraints: const BoxConstraints(minWidth: 112),
        padding: EdgeInsets.symmetric(
          horizontal: compact ? 12 : 14,
          vertical: verticalPadding,
        ),
        decoration: BoxDecoration(
          color: ApColors.surface1,
          borderRadius: BorderRadius.circular(7),
          border: Border.all(color: ApColors.borderMuted),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                if (icon != null) ...[
                  Icon(icon, size: 14, color: tint ?? ApColors.textMuted),
                  const SizedBox(width: 6),
                ],
                Flexible(
                  child: Text(
                    label.toUpperCase(),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: ApTypography.micro.copyWith(letterSpacing: .45),
                  ),
                ),
              ],
            ),
            SizedBox(height: sectionGap),
            Row(
              crossAxisAlignment: CrossAxisAlignment.baseline,
              textBaseline: TextBaseline.alphabetic,
              children: [
                Flexible(
                  child: Text(
                    value,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: compact ? 19 : 22,
                      height: 1.15,
                      fontWeight: FontWeight.w600,
                      color: tint ?? ApColors.textPrimary,
                    ),
                  ),
                ),
                if (unit != null) ...[
                  const SizedBox(width: 5),
                  Text(
                    unit!,
                    style: ApTypography.small.copyWith(
                      color: ApColors.textMuted,
                    ),
                  ),
                ],
              ],
            ),
            if (quality != null) ...[
              SizedBox(height: sectionGap),
              ApQualityBadge(quality: quality!),
            ],
          ],
        ),
      );
    },
  );
}

class ApSectionHeader extends StatelessWidget {
  const ApSectionHeader(this.title, {super.key, this.subtitle, this.trailing});
  final String title;
  final String? subtitle;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) => Row(
    children: [
      Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: ApTypography.sectionTitle),
            if (subtitle != null)
              Text(
                subtitle!,
                style: ApTypography.small.copyWith(color: ApColors.textMuted),
              ),
          ],
        ),
      ),
      ?trailing,
    ],
  );
}

class ApEmptyState extends StatelessWidget {
  const ApEmptyState({
    super.key,
    required this.title,
    this.message,
    this.action,
    this.icon = Icons.inbox_outlined,
  });
  final String title;
  final String? message;
  final Widget? action;
  final IconData icon;

  @override
  Widget build(BuildContext context) => Center(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, color: ApColors.textMuted, size: 28),
          const SizedBox(height: 10),
          Text(title, style: ApTypography.sectionTitle),
          if (message != null) ...[
            const SizedBox(height: 4),
            Text(
              message!,
              textAlign: TextAlign.center,
              style: ApTypography.small.copyWith(color: ApColors.textMuted),
            ),
          ],
          if (action != null) ...[const SizedBox(height: 12), action!],
        ],
      ),
    ),
  );
}

class ApErrorState extends StatelessWidget {
  const ApErrorState({super.key, required this.message, this.onRetry});
  final String message;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(12),
    decoration: BoxDecoration(
      color: ApColors.danger.withValues(alpha: .08),
      border: Border.all(color: ApColors.danger.withValues(alpha: .28)),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Row(
      children: [
        const Icon(Icons.error_outline, color: ApColors.danger, size: 17),
        const SizedBox(width: 8),
        Expanded(child: Text(message, style: ApTypography.small)),
        if (onRetry != null)
          TextButton(onPressed: onRetry, child: const Text('重试')),
      ],
    ),
  );
}

class ApCodeValue extends StatelessWidget {
  const ApCodeValue(this.value, {super.key, this.maxLines = 1});
  final String value;
  final int maxLines;
  @override
  Widget build(BuildContext context) =>
      SelectableText(value, maxLines: maxLines, style: ApTypography.code);
}

class ApMetricStrip extends StatelessWidget {
  const ApMetricStrip({
    super.key,
    required this.children,
    this.minimumTileWidth = 150,
  });
  final List<Widget> children;
  final double minimumTileWidth;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final count = (constraints.maxWidth / minimumTileWidth)
          .floor()
          .clamp(1, children.length)
          .toInt();
      return GridView.count(
        crossAxisCount: count,
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        crossAxisSpacing: 8,
        mainAxisSpacing: 8,
        childAspectRatio: count == 1 ? 4.0 : 1.65,
        children: children,
      );
    },
  );
}

class ApFilterBar extends StatelessWidget {
  const ApFilterBar({
    super.key,
    required this.children,
    this.onReset,
    this.resetLabel = 'Reset',
  });
  final List<Widget> children;
  final VoidCallback? onReset;
  final String resetLabel;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(10),
    decoration: BoxDecoration(
      color: ApColors.surface1,
      borderRadius: BorderRadius.circular(7),
      border: Border.all(color: ApColors.borderMuted),
    ),
    child: Wrap(
      spacing: 10,
      runSpacing: 8,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        ...children,
        if (onReset != null)
          TextButton(onPressed: onReset, child: Text(resetLabel)),
      ],
    ),
  );
}

class ApInspector extends StatelessWidget {
  const ApInspector({
    super.key,
    required this.title,
    required this.child,
    this.width = 320,
  });
  final String title;
  final Widget child;
  final double width;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: width,
    child: ApPanel(
      title: title,
      padding: const EdgeInsets.all(14),
      child: child,
    ),
  );
}

class ApInlineNotice extends StatelessWidget {
  const ApInlineNotice({
    super.key,
    required this.message,
    this.tone = ApTone.info,
    this.icon,
  });
  final String message;
  final ApTone tone;
  final IconData? icon;
  @override
  Widget build(BuildContext context) {
    final color = apToneColor(tone);
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .08),
        border: Border.all(color: color.withValues(alpha: .25)),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Row(
        children: [
          Icon(icon ?? Icons.info_outline, size: 16, color: color),
          const SizedBox(width: 8),
          Expanded(child: Text(message, style: ApTypography.small)),
        ],
      ),
    );
  }
}

class ApTimeRangeSelector extends StatelessWidget {
  const ApTimeRangeSelector({
    super.key,
    required this.value,
    required this.onChanged,
  });
  final String value;
  final ValueChanged<String> onChanged;
  @override
  Widget build(BuildContext context) => DropdownButton<String>(
    value: value,
    isDense: true,
    items: const [
      DropdownMenuItem(value: '15m', child: Text('15 min')),
      DropdownMenuItem(value: '1h', child: Text('1 hour')),
      DropdownMenuItem(value: '6h', child: Text('6 hours')),
      DropdownMenuItem(value: '24h', child: Text('24 hours')),
      DropdownMenuItem(value: '7d', child: Text('7 days')),
    ],
    onChanged: (next) {
      if (next != null) onChanged(next);
    },
  );
}

class ApLoadingState extends StatelessWidget {
  const ApLoadingState({super.key, this.label = 'Loading'});
  final String label;
  @override
  Widget build(BuildContext context) => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const SizedBox(
          width: 20,
          height: 20,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
        const SizedBox(height: 10),
        Text(label, style: ApTypography.small),
      ],
    ),
  );
}

class ApSplitView extends StatelessWidget {
  const ApSplitView({
    super.key,
    required this.primary,
    required this.secondary,
    this.secondaryWidth = 320,
    this.showSecondary = true,
  });
  final Widget primary;
  final Widget secondary;
  final double secondaryWidth;
  final bool showSecondary;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final show = showSecondary && constraints.maxWidth >= 1050;
      return Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Expanded(child: primary),
          if (show) ...[
            const SizedBox(width: 12),
            SizedBox(width: secondaryWidth, child: secondary),
          ],
        ],
      );
    },
  );
}

String apCompactNumber(Object? value) {
  if (value is! num) return '—';
  if (value.abs() >= 1000000) return '${(value / 1000000).toStringAsFixed(1)}m';
  if (value.abs() >= 10000) return '${(value / 1000).toStringAsFixed(1)}k';
  return value % 1 == 0 ? value.toInt().toString() : value.toStringAsFixed(1);
}

String apDuration(dynamic value) {
  if (value is! num || !value.isFinite || value < 0) return '—';
  if (value < 1000) return '${value.toStringAsFixed(0)} ms';
  final seconds = value / 1000;
  if (seconds < 60) return '${seconds.toStringAsFixed(1)} s';
  final minutes = seconds / 60;
  if (minutes < 60) return '${minutes.floor()}m ${(minutes % 1 * 60).round()}s';
  return '${(minutes / 60).toStringAsFixed(1)} h';
}
