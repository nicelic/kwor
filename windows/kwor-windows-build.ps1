# PowerShell script for building kwor on Windows
param(
    [string]$Architecture = "amd64",
    [switch]$NoCGO,
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: .\kwor-windows-build.ps1 [-Architecture <arch>] [-NoCGO] [-Help]"
    Write-Host "Architectures: amd64, arm64"
    Write-Host "Examples:"
    Write-Host "  .\kwor-windows-build.ps1                     # Build for amd64"
    Write-Host "  .\kwor-windows-build.ps1 -Architecture arm64 # Build for Windows arm64"
    Write-Host "  .\kwor-windows-build.ps1 -NoCGO              # Accepted for compatibility; build is already pure Go"
    exit 0
}

if ($NoCGO) {
    Write-Host "Note: kwor Windows builds already use CGO_ENABLED=0." -ForegroundColor Yellow
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

Write-Host "Building kwor for Windows ($Architecture)..." -ForegroundColor Green

# Check if Go is installed
try {
    $goVersion = go version 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "Go not found"
    }
    Write-Host "Go version: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "Error: Go is not installed or not in PATH" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# Check if Node.js / npm.cmd are installed
try {
    $nodeVersion = node --version 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "Node.js not found"
    }
    $null = Get-Command npm.cmd -ErrorAction Stop
    Write-Host "Node.js version: $nodeVersion" -ForegroundColor Green
} catch {
    Write-Host "Error: Node.js/npm.cmd is not installed or not in PATH" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# Build frontend
Write-Host "Building frontend..." -ForegroundColor Yellow
Push-Location temp_frontend

try {
    Write-Host "Installing dependencies..." -ForegroundColor Cyan
    npm.cmd install
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install frontend dependencies"
    }

    Write-Host "Building frontend..." -ForegroundColor Cyan
    npm.cmd run build
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to build frontend"
    }
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
    Pop-Location
    Read-Host "Press Enter to exit"
    exit 1
}

Pop-Location

# Create web/html directory
Write-Host "Creating web/html directory..." -ForegroundColor Yellow
if (Test-Path "web\html") {
    Remove-Item "web\html" -Recurse -Force
}
New-Item -ItemType Directory -Path "web\html" -Force | Out-Null

# Copy frontend build files
Write-Host "Copying frontend build files..." -ForegroundColor Yellow
Copy-Item "temp_frontend\dist\*" "web\html\" -Recurse -Force

# Build backend
Write-Host "Building backend (pure Go, CGO disabled)..." -ForegroundColor Yellow

# Set environment variables
$env:GOOS = "windows"
$env:GOARCH = $Architecture
$env:CGO_ENABLED = "0"

# Build command
$buildCmd = "go build -ldflags `"-w -s`" -o kwor.exe main.go"

try {
    Invoke-Expression $buildCmd
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to build backend"
    }
    Write-Host "Built successfully without CGO" -ForegroundColor Green
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

Write-Host "Build completed successfully!" -ForegroundColor Green
Write-Host "Output: kwor.exe" -ForegroundColor Green

# Show file info
if (Test-Path "kwor.exe") {
    $fileInfo = Get-Item "kwor.exe"
    Write-Host "File size: $([math]::Round($fileInfo.Length / 1MB, 2)) MB" -ForegroundColor Cyan
    Write-Host "Created: $($fileInfo.CreationTime)" -ForegroundColor Cyan
}

Read-Host "Press Enter to exit"
