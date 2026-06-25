@echo off
setlocal
chcp 65001 >nul 2>&1
cd /d "%~dp0\.."

echo ============================================
echo    Building kwor for Windows
echo ============================================
echo.

where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go is not installed or not in PATH.
    pause
    exit /b 1
)

where node >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js is not installed or not in PATH.
    pause
    exit /b 1
)

where npm.cmd >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] npm.cmd is not installed or not in PATH.
    pause
    exit /b 1
)

if "%GOARCH%"=="" set "GOARCH=amd64"

echo [1/3] Building frontend from temp_frontend...
pushd temp_frontend
call npm.cmd install
if %ERRORLEVEL% NEQ 0 (
    echo [FAILED] npm install failed.
    popd
    pause
    exit /b 1
)
call npm.cmd run build
if %ERRORLEVEL% NEQ 0 (
    echo [FAILED] Frontend build failed.
    popd
    pause
    exit /b 1
)
popd
echo [1/3] Frontend build complete.
echo.

echo [2/3] Copying frontend to web\html...
if exist web\html rd /s /q web\html
mkdir web\html
robocopy temp_frontend\dist web\html /MIR /NFL /NDL /NJH /NJS /NP >nul
set "COPY_RESULT=%ERRORLEVEL%"
if %COPY_RESULT% GEQ 8 (
    echo [FAILED] Copy frontend files failed.
    pause
    exit /b 1
)
echo [2/3] Frontend files copied.
echo.

echo [3/3] Building kwor.exe with CGO disabled...
set GOOS=windows
set CGO_ENABLED=0
go build -ldflags="-w -s" -o kwor.exe main.go
if %ERRORLEVEL% NEQ 0 (
    echo [FAILED] Go build failed for Windows %GOARCH%.
    pause
    exit /b 1
)
echo [3/3] Build complete.
echo.

echo Output: %CD%\kwor.exe
pause
