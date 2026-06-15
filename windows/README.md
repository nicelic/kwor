# Windows Files

This directory contains Windows-specific helper files for kwor local development and optional manual packaging.

## Available Files:

- **kwor-windows.xml**: WinSW service configuration used by the Windows installer
- **install-windows.bat**: Installation script
- **kwor-windows.bat**: Control panel script
- **uninstall-windows.bat**: Uninstallation script
- **kwor-windows-build.bat**: Simple build script for CMD
- **kwor-windows-build.ps1**: Build script for PowerShell

## Usage:

To install kwor on Windows:
1. Run `install-windows.bat` as Administrator
2. Follow the installation wizard
3. Use `kwor-windows.bat` in the install directory for management

To build from source:
- With CMD: `kwor-windows-build.bat`
- With PowerShell: `.\kwor-windows-build.ps1`
