import 'package:flutter/material.dart';

import '../ui/theme/ap_theme.dart';
import 'app_bootstrap.dart';

class AgentPulseApp extends StatelessWidget {
  const AgentPulseApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'AgentPulse',
    debugShowCheckedModeBanner: false,
    theme: ApTheme.dark,
    home: const AppBootstrap(),
  );
}
