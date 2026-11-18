@echo off
REM Builds the client and server into dist\ (needs Go 1.24+ from https://go.dev/dl/)
cd /d "%~dp0\.."
if not exist dist mkdir dist
set /p VERSION=<VERSION
set VX=-X github.com/Cmacle/TheHouseAlwaysWins/internal/server.Version=%VERSION% -X github.com/Cmacle/TheHouseAlwaysWins/internal/client.Version=%VERSION%
REM icon, version info and manifest (antivirus is much happier with them)
go run .\tools\winres -version %VERSION% -name "The House Always Wins" -exe TheHouseAlwaysWins.exe -out cmd\client\rsrc_windows_amd64.syso || goto :err
go run .\tools\winres -version %VERSION% -name "The House Always Wins Server" -exe HouseServer.exe -out cmd\server\rsrc_windows_amd64.syso || goto :err
go build -trimpath -ldflags "-H windowsgui %VX%" -o dist\TheHouseAlwaysWins.exe .\cmd\client || goto :err
go build -trimpath -ldflags "%VX%" -o dist\HouseServer.exe .\cmd\server || goto :err
copy /y scripts\run-server.bat dist\ >nul
copy /y LICENSE dist\ >nul
copy /y THIRD_PARTY_NOTICES.md dist\ >nul
echo Built dist\TheHouseAlwaysWins.exe and dist\HouseServer.exe
goto :eof
:err
echo Build failed.
exit /b 1
