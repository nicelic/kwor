@echo off
setlocal EnableExtensions DisableDelayedExpansion
chcp 65001 >nul 2>&1
cd /d "%~dp0"

set "RELEASE_DIR=%CD%\releases"
set "PACKAGE_TEMP="
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
where tar.exe >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Windows tar.exe is required to create release archives!
    pause
    exit /b 1
)
where powershell.exe >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Windows PowerShell is required to create the source zip!
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

:: Step 1: Install frontend dependencies (if needed) and build.
echo [1/5] Building frontend...
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
echo [1/5] Frontend build complete.
echo.

:: Step 2: The frontend build synchronizes web/html for Go embedding.
echo [2/5] Verifying embedded frontend sync ...
if not exist web\html\index.html (
    echo.
    echo [FAILED] Embedded frontend output is missing!
    pause
    exit /b 1
)
echo [2/5] Embedded frontend is synchronized.
echo.

:: Step 3: Prepare the six release assets. README.md is retained; other root-level Markdown files are excluded.
echo [3/5] Preparing release directory...
if not exist "%RELEASE_DIR%" mkdir "%RELEASE_DIR%"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create release directory failed!
    pause
    exit /b 1
)

set "VERSION="
set /p VERSION=<config\version
if not defined VERSION (
    echo.
    echo [FAILED] config\version is empty!
    pause
    exit /b 1
)

del /q "%RELEASE_DIR%\User Manual.md" >nul 2>&1
del /q "%RELEASE_DIR%\使用手册.md" >nul 2>&1
del /q "%RELEASE_DIR%\kwor_amd64" >nul 2>&1
del /q "%RELEASE_DIR%\kwor_arm64" >nul 2>&1
del /q "%RELEASE_DIR%\kwor-linux-amd64.tar.gz" >nul 2>&1
del /q "%RELEASE_DIR%\kwor-linux-arm64.tar.gz" >nul 2>&1
del /q "%RELEASE_DIR%\kwor-v*-source.tar.gz" >nul 2>&1
del /q "%RELEASE_DIR%\kwor-v*-source.zip" >nul 2>&1

set "PACKAGE_TEMP=%TEMP%\kwor-release-%RANDOM%-%RANDOM%-%RANDOM%"
mkdir "%PACKAGE_TEMP%"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create temporary package directory failed!
    pause
    exit /b 1
)

set "SOURCE_DIR_NAME=kwor-v%VERSION%"
set "SOURCE_ROOT=%PACKAGE_TEMP%\source"
set "SOURCE_STAGE=%SOURCE_ROOT%\%SOURCE_DIR_NAME%"
echo [3/5] Release directory ready: %RELEASE_DIR%
echo.

:: Step 4: Build Go binaries for Linux amd64 and arm64.
echo [4/5] Compiling Go binary (Linux amd64)...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-w -s" -o "%RELEASE_DIR%\kwor_amd64" main.go
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Go build failed for Linux amd64!
    goto :failed
)
echo [4/5] Linux amd64 binary compiled.
echo.

echo [4/5] Compiling Go binary (Linux arm64)...
set GOARCH=arm64
go build -ldflags="-w -s" -o "%RELEASE_DIR%\kwor_arm64" main.go
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Go build failed for Linux arm64!
    goto :failed
)
echo [4/5] Linux arm64 binary compiled.
echo.

:: Step 5: Create binary and source archives.
echo [5/5] Packaging release assets...
mkdir "%PACKAGE_TEMP%\kwor-linux-amd64"
mkdir "%PACKAGE_TEMP%\kwor-linux-arm64"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create binary package directories failed!
    goto :failed
)

copy /y "%RELEASE_DIR%\kwor_amd64" "%PACKAGE_TEMP%\kwor-linux-amd64\kwor" >nul
copy /y "%RELEASE_DIR%\kwor_arm64" "%PACKAGE_TEMP%\kwor-linux-arm64\kwor" >nul
copy /y "kwor.service" "%PACKAGE_TEMP%\kwor-linux-amd64\kwor.service" >nul
copy /y "kwor.service" "%PACKAGE_TEMP%\kwor-linux-arm64\kwor.service" >nul
copy /y "install.sh" "%PACKAGE_TEMP%\kwor-linux-amd64\install.sh" >nul
copy /y "install.sh" "%PACKAGE_TEMP%\kwor-linux-arm64\install.sh" >nul
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Prepare binary package files failed!
    goto :failed
)

tar.exe -czf "%RELEASE_DIR%\kwor-linux-amd64.tar.gz" -C "%PACKAGE_TEMP%\kwor-linux-amd64" .
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create Linux amd64 archive failed!
    goto :failed
)
tar.exe -czf "%RELEASE_DIR%\kwor-linux-arm64.tar.gz" -C "%PACKAGE_TEMP%\kwor-linux-arm64" .
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create Linux arm64 archive failed!
    goto :failed
)

mkdir "%SOURCE_ROOT%"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create source package directory failed!
    goto :failed
)

setlocal EnableDelayedExpansion
set "SOURCE_EXCLUDE_DIRS="%RELEASE_DIR%" "%CD%\release" "%CD%\tmp" "%CD%\temp_frontend\node_modules" "%CD%\temp_frontend\dist" "%CD%\web\html" "%CD%\Promanager_data" "%CD%\db" "%CD%\bin" "%CD%\backup" "%CD%\.git" "%CD%\.vscode" "%CD%\.cache" "%CD%\.claude" "%CD%\.codex" "%CD%\tools""
for /d %%D in ("%CD%\.release-work-*") do set "SOURCE_EXCLUDE_DIRS=!SOURCE_EXCLUDE_DIRS! "%%~fD""
for /d %%D in ("%CD%\.shadowquic-build-*") do set "SOURCE_EXCLUDE_DIRS=!SOURCE_EXCLUDE_DIRS! "%%~fD""
robocopy "%CD%" "%SOURCE_STAGE%" /E /COPY:DAT /DCOPY:DAT /R:1 /W:1 ^
    /XD !SOURCE_EXCLUDE_DIRS! ^
    /XF "*.exe" "*.tar.gz" "*.zip" "gcm-diagnose.log" "kwor" "sui" "main" >nul
set "ROBOCOPY_EXIT=!ERRORLEVEL!"
endlocal & set "ROBOCOPY_EXIT=%ROBOCOPY_EXIT%"
if %ROBOCOPY_EXIT% GEQ 8 (
    echo.
    echo [FAILED] Copy source files for archive failed!
    goto :failed
)

if not exist "README.md" (
    echo.
    echo [FAILED] Root README.md is required for source archives!
    goto :failed
)

del /q "%SOURCE_STAGE%\*.md" >nul 2>&1
copy /y "README.md" "%SOURCE_STAGE%\README.md" >nul
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Preserve README.md in source archive failed!
    goto :failed
)
if not exist "%SOURCE_STAGE%\README.md" (
    echo.
    echo [FAILED] Source archive staging is missing README.md!
    goto :failed
)

tar.exe -czf "%RELEASE_DIR%\kwor-v%VERSION%-source.tar.gz" -C "%SOURCE_ROOT%" "%SOURCE_DIR_NAME%"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create source tar.gz failed!
    goto :failed
)

set "KWOR_SOURCE_STAGE=%SOURCE_STAGE%"
set "KWOR_SOURCE_ZIP=%RELEASE_DIR%\kwor-v%VERSION%-source.zip"
powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; Add-Type -AssemblyName System.IO.Compression.FileSystem; [System.IO.Compression.ZipFile]::CreateFromDirectory($env:KWOR_SOURCE_STAGE, $env:KWOR_SOURCE_ZIP, [System.IO.Compression.CompressionLevel]::Optimal, $true)"
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [FAILED] Create source zip failed!
    goto :failed
)

tar.exe -tzf "%RELEASE_DIR%\kwor-linux-amd64.tar.gz" >nul
if %ERRORLEVEL% NEQ 0 goto :archive_failed
tar.exe -tzf "%RELEASE_DIR%\kwor-linux-arm64.tar.gz" >nul
if %ERRORLEVEL% NEQ 0 goto :archive_failed
tar.exe -tzf "%RELEASE_DIR%\kwor-v%VERSION%-source.tar.gz" >nul
if %ERRORLEVEL% NEQ 0 goto :archive_failed
tar.exe -tzf "%RELEASE_DIR%\kwor-v%VERSION%-source.tar.gz" | findstr.exe /x /c:"%SOURCE_DIR_NAME%/README.md" >nul
if %ERRORLEVEL% NEQ 0 goto :archive_failed
powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; Add-Type -AssemblyName System.IO.Compression.FileSystem; $zip = [System.IO.Compression.ZipFile]::OpenRead($env:KWOR_SOURCE_ZIP); try { if (-not ($zip.Entries.FullName -contains ($env:SOURCE_DIR_NAME + '\README.md'))) { throw 'Source zip is missing README.md.' } } finally { $zip.Dispose() }"
if %ERRORLEVEL% NEQ 0 goto :archive_failed

rmdir /s /q "%PACKAGE_TEMP%" >nul 2>&1
set "PACKAGE_TEMP="

echo.
echo ============================================
echo    Build successful!
echo ============================================
echo.
echo Output files:
dir /b "%RELEASE_DIR%\kwor_amd64"
dir /b "%RELEASE_DIR%\kwor_arm64"
dir /b "%RELEASE_DIR%\kwor-linux-amd64.tar.gz"
dir /b "%RELEASE_DIR%\kwor-linux-arm64.tar.gz"
dir /b "%RELEASE_DIR%\kwor-v%VERSION%-source.tar.gz"
dir /b "%RELEASE_DIR%\kwor-v%VERSION%-source.zip"
echo.
echo Binary archives contain kwor, kwor.service, and install.sh.
echo Source archives contain README.md and docs, and exclude other root-level Markdown files.
echo.
pause
exit /b 0

:archive_failed
echo.
echo [FAILED] Archive verification failed!
goto :failed

:failed
if defined PACKAGE_TEMP if exist "%PACKAGE_TEMP%" rmdir /s /q "%PACKAGE_TEMP%" >nul 2>&1
pause
exit /b 1
