#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <flutter_windows.h>
#include <windows.h>

#include <algorithm>
#include "flutter_window.h"
#include "utils.h"

int APIENTRY wWinMain(_In_ HINSTANCE instance, _In_opt_ HINSTANCE prev,
                      _In_ wchar_t *command_line, _In_ int show_command) {
  // Attach to console when present (e.g., 'flutter run') or create a
  // new console when running with a debugger.
  if (!::AttachConsole(ATTACH_PARENT_PROCESS) && ::IsDebuggerPresent()) {
    CreateAndAttachConsole();
  }

  // Initialize COM, so that it is available for use in the library and/or
  // plugins.
  ::CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);

  flutter::DartProject project(L"data");

  std::vector<std::string> command_line_arguments =
      GetCommandLineArguments();

  project.set_dart_entrypoint_arguments(std::move(command_line_arguments));

  FlutterWindow window(project);
  Win32Window::Point origin(10, 10);
  Win32Window::Size size(1440, 900);
  const POINT monitor_point = {0, 0};
  HMONITOR monitor = MonitorFromPoint(monitor_point, MONITOR_DEFAULTTOPRIMARY);
  MONITORINFO monitor_info{};
  monitor_info.cbSize = sizeof(monitor_info);
  if (monitor != nullptr && GetMonitorInfo(monitor, &monitor_info)) {
    const UINT dpi = FlutterDesktopGetDpiForMonitor(monitor);
    const double scale_factor = dpi > 0 ? dpi / 96.0 : 1.0;
    const int work_width = static_cast<int>(
        (monitor_info.rcWork.right - monitor_info.rcWork.left) / scale_factor);
    const int work_height = static_cast<int>(
        (monitor_info.rcWork.bottom - monitor_info.rcWork.top) / scale_factor);
    size.width = std::min(size.width,
        static_cast<unsigned int>(std::max(1100, work_width - 20)));
    size.height = std::min(size.height,
        static_cast<unsigned int>(std::max(720, work_height - 20)));
    const int work_left = static_cast<int>(monitor_info.rcWork.left / scale_factor);
    const int work_top = static_cast<int>(monitor_info.rcWork.top / scale_factor);
    origin.x = static_cast<unsigned int>(work_left + std::max(0, (work_width - static_cast<int>(size.width)) / 2));
    origin.y = static_cast<unsigned int>(work_top + std::max(0, (work_height - static_cast<int>(size.height)) / 2));
  }
  if (!window.Create(L"agentpulse", origin, size)) {
    return EXIT_FAILURE;
  }
  window.SetQuitOnClose(true);

  ::MSG msg;
  while (::GetMessage(&msg, nullptr, 0, 0)) {
    ::TranslateMessage(&msg);
    ::DispatchMessage(&msg);
  }

  ::CoUninitialize();
  return EXIT_SUCCESS;
}
