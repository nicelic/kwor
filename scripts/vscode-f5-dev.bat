@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1
cd /d "%~dp0\.."

set "DEV_BIN_DIR=%CD%\.vscode\bin"
set "DEV_EXE=%DEV_BIN_DIR%\kwor-dev.exe"

if exist "%LOCALAPPDATA%\Programs\nodejs\node.exe" set "PATH=%LOCALAPPDATA%\Programs\nodejs;%PATH%"

tasklist /FI "IMAGENAME eq kwor-dev.exe" 2>nul | find /I "kwor-dev.exe" >nul
if %ERRORLEVEL% EQU 0 (
    echo [ERROR] kwor-dev.exe is already running. Stop the current F5 debug session first.
    exit /b 1
)

where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go is not installed or not in PATH.
    exit /b 1
)

where node >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js is not installed or not in PATH.
    exit /b 1
)

where npm.cmd >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] npm.cmd is not installed or not in PATH.
    exit /b 1
)

echo [1/5] Building frontend with npm.cmd for debug...
pushd temp_frontend
if not exist node_modules (
    echo Installing frontend dependencies...
    call npm.cmd install
    if %ERRORLEVEL% NEQ 0 (
        popd
        echo [ERROR] npm.cmd install failed.
        exit /b 1
    )
)
call npm.cmd run build:debug
if %ERRORLEVEL% NEQ 0 (
    popd
    echo [ERROR] npm.cmd run build:debug failed.
    exit /b 1
)
popd

echo [2/5] Copying frontend assets to web\html...
if exist "web\html" rd /s /q "web\html"
mkdir "web\html"
xcopy /s /e /y "temp_frontend\dist\*" "web\html\" >nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Copy frontend assets failed.
    exit /b 1
)

echo [3/5] Building Windows debug backend...
if not exist "%DEV_BIN_DIR%" mkdir "%DEV_BIN_DIR%"
set "GOOS="
set "GOARCH="
set "CGO_ENABLED="
go build -gcflags="all=-N -l" -o "%DEV_EXE%" main.go
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go debug build failed.
    exit /b 1
)

echo [4/5] Preparing local debug panel settings...
"%DEV_EXE%" setting -port 8888 -path /app
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Prepare panel settings failed.
    exit /b 1
)

echo [5/5] Preparing local debug admin credentials...
"%DEV_EXE%" admin -username 000000 -password 000000
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Prepare admin credentials failed.
    exit /b 1
)

echo [OK] F5 debug build is ready: http://127.0.0.1:8888/app
exit /b 0
