@echo off
REM Builds the client and server into dist\ (needs Go 1.24+ from https://go.dev/dl/)
cd /d "%~dp0\.."
if not exist dist mkdir dist
go build -trimpath -ldflags "-H windowsgui -s -w" -o dist\TheHouseAlwaysWins.exe .\cmd\client || goto :err
go build -trimpath -ldflags "-s -w" -o dist\HouseServer.exe .\cmd\server || goto :err
copy /y scripts\run-server.bat dist\ >nul
echo Built dist\TheHouseAlwaysWins.exe and dist\HouseServer.exe
goto :eof
:err
echo Build failed.
exit /b 1
