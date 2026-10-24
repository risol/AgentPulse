import 'package:flutter/material.dart';

import 'ap_colors.dart';

abstract final class ApTypography {
  static const sans = 'Segoe UI Variable';
  static const mono = 'Cascadia Mono';

  static const pageTitle = TextStyle(
    fontFamily: sans,
    fontSize: 24,
    height: 30 / 24,
    fontWeight: FontWeight.w600,
    color: ApColors.textPrimary,
  );
  static const sectionTitle = TextStyle(
    fontFamily: sans,
    fontSize: 15,
    height: 22 / 15,
    fontWeight: FontWeight.w600,
    color: ApColors.textPrimary,
  );
  static const body = TextStyle(
    fontFamily: sans,
    fontSize: 13,
    height: 20 / 13,
    color: ApColors.textPrimary,
  );
  static const small = TextStyle(
    fontFamily: sans,
    fontSize: 12,
    height: 18 / 12,
    color: ApColors.textSecondary,
  );
  static const micro = TextStyle(
    fontFamily: sans,
    fontSize: 11,
    height: 16 / 11,
    color: ApColors.textMuted,
  );
  static const code = TextStyle(
    fontFamily: mono,
    fontSize: 12,
    height: 18 / 12,
    color: ApColors.textPrimary,
  );
}
