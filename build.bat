@echo off
setlocal
chcp 65001 >nul 2>&1
cd /d "%~dp0"
set "RELEASE_DIR=%CD%\releases"
if exist "%LOCALAPPDATA%\Programs\nodejs\node.exe" set "PATH=%LOCALAPPDATA%\Programs\nodejs;%PATH%"
echo ============================================
echo    Building kwor for Linux amd64 and arm64
echo ============================================
echo.

:: Check prerequisites
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go is not installed or not in PATH!
    pause
    exit /b 1
)
where node >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js is not installed or not in PATH!
    pause
    exit /b 1
)
where npm.cmd >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] npm is not installed or not in PATH!
    pause
    exit /b 1
)

echo Current Go version:
go version
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to get Go version!
    pause
    exit /b 1
)
echo.

:: Step 1: Install frontend dependencies (if needed) and build
echo [1/4] Building frontend...
pushd temp_frontend
if not exist node_modules (
    echo      Installing dependencies...
    call npm.cmd install
    if %ERRORLEVEL% NEQ 0 (
        echo.
        echo [FAILED] npm install failed!
        popd
        pause
        exit /b 1
    )
)
call npm.cmd run build
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Frontend build failed!
    popd
    pause
    exit /b 1
)
popd
echo [1/4] Frontend build complete.
echo.

:: Step 2: Copy frontend build output to web/html/
echo [2/4] Copying frontend to web/html/ ...
if exist web\html rd /s /q web\html
mkdir web\html
robocopy temp_frontend\dist web\html /MIR /NFL /NDL /NJH /NJS /NP >nul
set "COPY_RESULT=%ERRORLEVEL%"
if %COPY_RESULT% GEQ 8 (
    echo.
    echo [FAILED] Copy frontend files failed!
    pause
    exit /b 1
)
echo [2/4] Frontend files copied.
echo.

:: Step 3: Prepare release output directory
echo [3/4] Preparing release directory...
if not exist "%RELEASE_DIR%" mkdir "%RELEASE_DIR%"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create release directory failed!
    pause
    exit /b 1
)
if not exist "User Manual.md" (
    echo.
    echo [FAILED] Missing User Manual.md in repository root!
    pause
    exit /b 1
)
if not exist "使用手册.md" (
    echo.
    echo [FAILED] Missing 使用手册.md in repository root!
    pause
    exit /b 1
)
echo [3/4] Release directory ready: %RELEASE_DIR%
echo      Repository root manuals verified: User Manual.md, 使用手册.md
echo.

:: Step 4: Build Go binaries for Linux amd64 and arm64
echo [4/4] Compiling Go binary (Linux amd64)...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-w -s" -o "%RELEASE_DIR%\kwor_amd64" main.go
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Go build failed for Linux amd64!
    pause
    exit /b 1
)
echo [4/4] Linux amd64 binary compiled.
echo.

echo [4/4] Compiling Go binary (Linux arm64)...
set GOARCH=arm64
go build -ldflags="-w -s" -o "%RELEASE_DIR%\kwor_arm64" main.go
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Go build failed for Linux arm64!
    pause
    exit /b 1
)
echo [4/4] Linux arm64 binary compiled.
echo.

:: Show result
echo ============================================
echo    Build successful!
echo ============================================
echo.
echo Output files:
dir /b "%RELEASE_DIR%\kwor_*"
echo.
echo Deploy to Linux:
echo   1. Upload the matching file from 'releases'
echo   2. Rename 'kwor_amd64' or 'kwor_arm64' to 'kwor'
echo   3. chmod +x kwor
echo   4. ./kwor start
echo.
pause
