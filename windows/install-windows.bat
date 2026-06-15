@echo off
setlocal enabledelayedexpansion

echo ========================================
echo kwor Windows Installer
echo ========================================

REM Check if running as Administrator
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Error: This script must be run as Administrator
    echo Right-click on this file and select "Run as administrator"
    pause
    exit /b 1
)

REM Set installation directory
set "INSTALL_DIR=C:\Program Files\kwor"
set "SERVICE_NAME=kwor"

echo Installing kwor to: %INSTALL_DIR%

REM Create installation directory
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"
if not exist "%INSTALL_DIR%\Promanager_data" mkdir "%INSTALL_DIR%\Promanager_data"
if not exist "%INSTALL_DIR%\Promanager_data\db" mkdir "%INSTALL_DIR%\Promanager_data\db"
if not exist "%INSTALL_DIR%\Promanager_data\core" mkdir "%INSTALL_DIR%\Promanager_data\core"
if not exist "%INSTALL_DIR%\Promanager_data\cert" mkdir "%INSTALL_DIR%\Promanager_data\cert"
if not exist "%INSTALL_DIR%\logs" mkdir "%INSTALL_DIR%\logs"

REM Copy files
echo Copying files...
copy "kwor.exe" "%INSTALL_DIR%\" >nul
copy "kwor-windows.xml" "%INSTALL_DIR%\kwor-service.xml" >nul
copy "kwor-windows.bat" "%INSTALL_DIR%\kwor-windows.bat" >nul
copy "uninstall-windows.bat" "%INSTALL_DIR%\" >nul
copy "README.md" "%INSTALL_DIR%\" >nul

REM Check if WinSW is available
set "WINSW_PATH=%INSTALL_DIR%\winsw.exe"
if not exist "%WINSW_PATH%" (
    echo Downloading WinSW...
    powershell -Command "& {Invoke-WebRequest -Uri 'https://github.com/winsw/winsw/releases/download/v2.12.0/WinSW-x64.exe' -OutFile '%WINSW_PATH%'}"
    if exist "%WINSW_PATH%" (
        echo WinSW downloaded successfully
    ) else (
        echo Warning: Failed to download WinSW. Service installation will be skipped.
        echo You can manually download WinSW from: https://github.com/winsw/winsw/releases
    )
)

REM Install Windows Service
if exist "%WINSW_PATH%" (
    echo Installing Windows Service...
    cd /d "%INSTALL_DIR%"
    copy "winsw.exe" "kwor-service.exe" >nul
        
    REM Install service
    kwor-service.exe install
    if %errorLevel% equ 0 (
        echo Service installed successfully
    ) else (
        echo Warning: Failed to install service. You can install it manually later.
    )
)

REM Run migration
echo Running database migration...
cd /d "%INSTALL_DIR%"
kwor.exe migrate
if %errorLevel% equ 0 (
    echo Migration completed successfully
) else (
    echo Warning: Migration failed or database is new
)

REM Get network configuration
echo.
echo ========================================
echo Network Configuration
echo ========================================

REM Get local IP addresses
echo Available IP addresses:
for /f "tokens=2 delims=:" %%i in ('ipconfig ^| findstr /i "IPv4"') do (
    echo   %%i
)

REM Get panel configuration
echo.
set /p panel_port="Enter panel port (default: 8888): "
if "%panel_port%"=="" set "panel_port=8888"

set /p panel_path="Enter panel path (default: /app/): "
if "%panel_path%"=="" set "panel_path=/app/"

set /p sub_port="Enter subscription port (default: 22780): "
if "%sub_port%"=="" set "sub_port=22780"

set /p sub_path="Enter subscription path (leave blank for auto-generated random path): "

REM Apply settings
echo.
echo Applying settings...
cd /d "%INSTALL_DIR%"
if "%sub_path%"=="" (
    kwor.exe setting -port %panel_port% -path "%panel_path%" -subPort %sub_port%
) else (
    kwor.exe setting -port %panel_port% -path "%panel_path%" -subPort %sub_port% -subPath "%sub_path%"
)

set "sub_path_display=%sub_path%"
if "%sub_path_display%"=="" set "sub_path_display=[auto-generated random path]"

REM Get admin credentials
echo.
echo ========================================
echo Admin Configuration
echo ========================================

set /p admin_username="Enter admin username (default: admin): "
if "%admin_username%"=="" set "admin_username=admin"

set /p admin_password="Enter admin password: "
if "%admin_password%"=="" (
    echo Error: Password cannot be empty
    pause
    exit /b 1
)

REM Set admin credentials
echo Setting admin credentials...
kwor.exe admin -username "%admin_username%" -password "%admin_password%"

echo.
echo Current settings:
kwor.exe setting -show
echo.
echo Current admin credentials:
kwor.exe admin -show

REM Start service
echo Starting kwor service...
net start %SERVICE_NAME%
if %errorLevel% equ 0 (
    echo Service started successfully
) else (
    echo Warning: Failed to start service. You can start it manually later.
)

REM Create desktop shortcut
echo Creating desktop shortcut...
set "DESKTOP=%USERPROFILE%\Desktop"
if exist "%DESKTOP%" (
    powershell -Command "& {$WshShell = New-Object -comObject WScript.Shell; $Shortcut = $WshShell.CreateShortcut('%DESKTOP%\kwor.lnk'); $Shortcut.TargetPath = '%INSTALL_DIR%\kwor-windows.bat'; $Shortcut.WorkingDirectory = '%INSTALL_DIR%'; $Shortcut.Description = 'kwor Control Panel'; $Shortcut.Save()}"
    echo Desktop shortcut created
)

REM Create Start Menu shortcut
echo Creating Start Menu shortcut...
set "START_MENU=%APPDATA%\Microsoft\Windows\Start Menu\Programs"
if exist "%START_MENU%" (
    if not exist "%START_MENU%\kwor" mkdir "%START_MENU%\kwor"
    powershell -Command "& {$WshShell = New-Object -comObject WScript.Shell; $Shortcut = $WshShell.CreateShortcut('%START_MENU%\kwor\kwor Control Panel.lnk'); $Shortcut.TargetPath = '%INSTALL_DIR%\kwor-windows.bat'; $Shortcut.WorkingDirectory = '%INSTALL_DIR%'; $Shortcut.Description = 'kwor Control Panel'; $Shortcut.Save()}"
    echo Start Menu shortcut created
)

REM Set permissions
echo Setting permissions...
icacls "%INSTALL_DIR%" /grant "Users:(OI)(CI)RX" /T >nul
icacls "%INSTALL_DIR%\Promanager_data" /grant "Users:(OI)(CI)F" /T >nul
icacls "%INSTALL_DIR%\logs" /grant "Users:(OI)(CI)F" /T >nul

REM Create environment variable
echo Setting environment variable...
setx KWOR_HOME "%INSTALL_DIR%" /M >nul

REM Show final configuration
echo.
echo ========================================
echo Installation completed successfully!
echo ========================================
echo.
echo kwor has been installed to: %INSTALL_DIR%
echo.
echo Configuration:
echo   Panel Port: %panel_port%
echo   Panel Path: %panel_path%
echo   Subscription Port: %sub_port%
echo   Subscription Path: %sub_path_display%
echo   Admin Username: %admin_username%
echo.
echo Access URLs:
for /f "tokens=2 delims=:" %%i in ('ipconfig ^| findstr /i "IPv4"') do (
    set "ip=%%i"
    set "ip=!ip: =!"
    echo   Panel: http://!ip!:%panel_port%%panel_path%
    echo   Subscription: http://!ip!:%sub_port%%sub_path_display%
)
echo.
echo Service name: %SERVICE_NAME%
echo.
echo Useful commands:
echo   net start %SERVICE_NAME%    - Start the service
echo   net stop %SERVICE_NAME%     - Stop the service
echo   sc query %SERVICE_NAME%     - Check service status
echo   kwor.exe uri                - Print panel URL
echo.
echo You can also use the desktop shortcut or Start Menu item.
echo.
pause
