import 'package:flutter/material.dart';

import '../theme/ap_colors.dart';
import '../theme/ap_typography.dart';
import 'ap_components.dart';

class ApDataTable extends StatelessWidget {
  const ApDataTable({
    super.key,
    required this.columns,
    required this.rows,
    this.sortColumnIndex,
    this.sortAscending = true,
    this.onSort,
    this.loading = false,
    this.emptyMessage = 'No records',
    this.footer,
  });
  final List<DataColumn> columns;
  final List<DataRow> rows;
  final int? sortColumnIndex;
  final bool sortAscending;
  final void Function(int, bool)? onSort;
  final bool loading;
  final String emptyMessage;
  final Widget? footer;

  @override
  Widget build(BuildContext context) => Container(
    decoration: BoxDecoration(
      color: ApColors.surface1,
      border: Border.all(color: ApColors.borderMuted),
      borderRadius: BorderRadius.circular(7),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (loading) const LinearProgressIndicator(minHeight: 2),
        if (rows.isEmpty && !loading)
          SizedBox(height: 180, child: ApEmptyState(title: emptyMessage)),
        if (rows.isNotEmpty)
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: DataTable(
              columns: columns,
              rows: rows,
              sortColumnIndex: sortColumnIndex,
              sortAscending: sortAscending,
              dataRowMinHeight: 36,
              dataRowMaxHeight: 40,
              headingRowHeight: 32,
              horizontalMargin: 12,
              columnSpacing: 24,
              headingTextStyle: ApTypography.micro.copyWith(
                color: ApColors.textMuted,
              ),
            ),
          ),
        if (footer != null)
          Padding(padding: const EdgeInsets.all(9), child: footer!),
      ],
    ),
  );
}
