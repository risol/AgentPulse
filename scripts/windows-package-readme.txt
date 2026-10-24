AgentPulse for Windows — offline installation bundle

Install or upgrade
1. Extract the ZIP file to a folder you can write to.
2. Close AgentPulse if it is running.
3. Right-click Install-AgentPulse.ps1 and choose Run with PowerShell.
   If Windows blocks the script, open PowerShell in this folder and run:
   powershell -NoProfile -ExecutionPolicy Bypass -File .\Install-AgentPulse.ps1
4. Start AgentPulse from the Start menu.

The Go launcher starts logging and the local receiver before opening the Flutter interface. Closing the interface shuts down the receiver.

The installer stores the application under your user profile and does not need administrator access.
Upgrades replace the application files in place. The SQLite database, logs, and Codex configuration are kept separately and are preserved.

Uninstall
Use “Uninstall AgentPulse” in the Start menu. The uninstaller removes the application and shortcuts but keeps your collection data at:
%APPDATA%\AgentPulse

To remove that data too, run the installed Uninstall-AgentPulse.ps1 with -RemoveUserData.
That deletes the local database and application logs. Export a database snapshot first if you may need it later.

The bundle runs locally and does not require an internet connection after it has been downloaded.
