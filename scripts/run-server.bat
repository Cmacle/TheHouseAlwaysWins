@echo off
REM Keeps the server running: restarts it if it ever exits.
cd /d "%~dp0"
REM tells the server this script restarts it (so after an update it just exits)
set HOUSE_SUPERVISED=1
:loop
HouseServer.exe
echo Server exited - restarting in 5 seconds (close this window to stop)...
timeout /t 5 /nobreak >nul
goto loop
