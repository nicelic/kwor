@echo off
setlocal enabledelayedexpansion

echo ========================================
echo kwor Windows Uninstaller
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

echo Uninstalling kwor from: %INSTALL_DIR%

REM Stop and remove Windows Service
if exist "%INSTALL_DIR%\kwor-service.exe" (
    echo Stopping and removing Windows Service...
    net stop %SERVICE_NAME% >nul 2>&1
    cd /d "%INSTALL_DIR%"
    kwor-service.exe uninstall >nul 2>&1
    if %errorLevel% equ 0 (
        echo Service removed successfully
    ) else (
        echo Warning: Failed to remove service or service was not installed
    )
)

REM Remove desktop shortcut
echo Removing desktop shortcut...
set "DESKTOP=%USERPROFILE%\Desktop"
if exist "%DESKTOP%\kwor.lnk" (
    del "%DESKTOP%\kwor.lnk" >nul 2>&1
    echo Desktop shortcut removed
)

REM Remove Start Menu shortcut
echo Removing Start Menu shortcut...
set "START_MENU=%APPDATA%\Microsoft\Windows\Start Menu\Programs\kwor"
if exist "%START_MENU%" (
    rmdir /s /q "%START_MENU%" >nul 2>&1
    echo Start Menu shortcut removed
)

REM Remove environment variable
echo Removing environment variable...
reg delete "HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v KWOR_HOME /f >nul 2>&1

REM Ask user if they want to keep data
echo.
set /p keep_data="Do you want to keep your data (database, logs, certificates)? [y/n]: "
if /i "%keep_data%"=="y" (
    echo Keeping data files...
    REM Remove only executable and service files
    if exist "%INSTALL_DIR%\kwor.exe" del "%INSTALL_DIR%\kwor.exe" >nul 2>&1
    if exist "%INSTALL_DIR%\kwor-service.exe" del "%INSTALL_DIR%\kwor-service.exe" >nul 2>&1
    if exist "%INSTALL_DIR%\kwor-service.xml" del "%INSTALL_DIR%\kwor-service.xml" >nul 2>&1
    if exist "%INSTALL_DIR%\winsw.exe" del "%INSTALL_DIR%\winsw.exe" >nul 2>&1
    if exist "%INSTALL_DIR%\kwor-windows.bat" del "%INSTALL_DIR%\kwor-windows.bat" >nul 2>&1
    if exist "%INSTALL_DIR%\install-windows.bat" del "%INSTALL_DIR%\install-windows.bat" >nul 2>&1
    if exist "%INSTALL_DIR%\uninstall-windows.bat" del "%INSTALL_DIR%\uninstall-windows.bat" >nul 2>&1
    if exist "%INSTALL_DIR%\README.md" del "%INSTALL_DIR%\README.md" >nul 2>&1
    echo Data files preserved in: %INSTALL_DIR%
) else (
    echo Removing all files...
    REM Remove entire installation directory
    if exist "%INSTALL_DIR%" (
        rmdir /s /q "%INSTALL_DIR%" >nul 2>&1
        if exist "%INSTALL_DIR%" (
            echo Warning: Some files could not be removed. Please manually delete: %INSTALL_DIR%
        ) else (
            echo All files removed successfully
        )
    )
)

REM Remove firewall rules
echo Removing firewall rules...
netsh advfirewall firewall delete rule name="kwor Panel" >nul 2>&1
netsh advfirewall firewall delete rule name="kwor Subscription" >nul 2>&1

echo.
echo ========================================
echo Uninstallation completed!
echo ========================================
echo.
echo kwor has been uninstalled from your system.
echo.
if /i "%keep_data%"=="y" (
    echo Your data has been preserved in: %INSTALL_DIR%
    echo You can safely delete this directory if you no longer need the data.
)
echo.
echo Thank you for using kwor!
echo.
pause
