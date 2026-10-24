import 'package:flutter/material.dart';

import 'ap_colors.dart';
import 'ap_typography.dart';

abstract final class ApTheme {
  static ThemeData get dark {
    const scheme = ColorScheme.dark(
      primary: ApColors.accent,
      onPrimary: Colors.white,
      secondary: ApColors.info,
      onSecondary: ApColors.bg,
      error: ApColors.danger,
      onError: Colors.white,
      surface: ApColors.surface1,
      onSurface: ApColors.textPrimary,
      outline: ApColors.border,
    );
    final base = ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      colorScheme: scheme,
      scaffoldBackgroundColor: ApColors.bg,
      fontFamily: ApTypography.sans,
      dividerColor: ApColors.borderMuted,
      textTheme: const TextTheme(
        displaySmall: ApTypography.pageTitle,
        titleLarge: ApTypography.pageTitle,
        titleMedium: ApTypography.sectionTitle,
        bodyLarge: ApTypography.body,
        bodyMedium: ApTypography.body,
        bodySmall: ApTypography.small,
        labelSmall: ApTypography.micro,
      ),
      visualDensity: VisualDensity.compact,
      splashFactory: NoSplash.splashFactory,
    );
    return base.copyWith(
      appBarTheme: const AppBarTheme(
        backgroundColor: ApColors.surface0,
        foregroundColor: ApColors.textPrimary,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: ApTypography.sectionTitle,
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: ApColors.surface0,
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 12,
          vertical: 10,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: ApColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: ApColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: ApColors.accent, width: 1.4),
        ),
        hintStyle: ApTypography.small.copyWith(color: ApColors.textMuted),
      ),
      dataTableTheme: const DataTableThemeData(
        headingRowColor: WidgetStatePropertyAll(ApColors.surface2),
        dataRowColor: WidgetStatePropertyAll(ApColors.surface1),
        dividerThickness: 1,
        headingTextStyle: ApTypography.micro,
        dataTextStyle: ApTypography.small,
        horizontalMargin: 12,
        columnSpacing: 24,
      ),
      tooltipTheme: TooltipThemeData(
        decoration: BoxDecoration(
          color: ApColors.surface3,
          borderRadius: BorderRadius.circular(6),
          border: Border.all(color: ApColors.border),
        ),
        textStyle: ApTypography.small,
        waitDuration: const Duration(milliseconds: 450),
      ),
      iconTheme: const IconThemeData(color: ApColors.textSecondary, size: 18),
      focusColor: ApColors.accent.withValues(alpha: .16),
      hoverColor: Colors.white.withValues(alpha: .035),
      snackBarTheme: const SnackBarThemeData(
        backgroundColor: ApColors.surface3,
        contentTextStyle: ApTypography.body,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }
}
