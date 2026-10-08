@echo off
rem The relo command for cmd and PowerShell. relo.exe is a GUI-subsystem
rem binary, so a window launch never flashes a console; cmd waits for a GUI
rem application inside a command script, which keeps output, pipes, and the
rem exit code intact here.
"%~dp0..\relo.exe" %*
exit /b %errorlevel%
