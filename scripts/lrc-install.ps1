# lrc installer for Windows PowerShell
# Usage: iwr -useb https://your-domain/lrc-install.ps1 | iex
#   or:  Invoke-WebRequest -Uri https://your-domain/lrc-install.ps1 -UseBasicParsing | Invoke-Expression

$ErrorActionPreference = "Stop"

# Plain ASCII status markers (Unicode chars show as ? in default Windows console)
$OK = "[OK]"
$FAIL = "[FAIL]"

function Print-ElevationHelp {
    Write-Host ""
    Write-Host "Troubleshooting:" -ForegroundColor Yellow
    Write-Host "  1) Try running PowerShell as Administrator (right-click -> Run as administrator)." -ForegroundColor Yellow
    Write-Host "  2) If UAC prompts fail, try a different terminal (some terminals do not prompt correctly)." -ForegroundColor Yellow
    Write-Host "  3) If admin access is not available, please file an issue: https://github.com/HexmosTech/LiveReview/issues" -ForegroundColor Yellow
    Write-Host ""
}

# URL where this script is hosted (used for self-elevation when piped)
$SCRIPT_URL = "https://hexmos.com/lrc-install.ps1"

# Self-elevate to admin if not already running as admin (like bash script requires sudo upfront)
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Requesting administrator privileges (required to install lrc)..." -ForegroundColor Yellow
    try {
        # Always download a fresh copy to temp for elevation (works for both piped and file execution)
        $scriptPath = "$env:TEMP\lrc-install-elevated.ps1"
        Write-Host "Downloading installer for elevated execution..." -ForegroundColor Yellow
        Invoke-WebRequest -Uri $SCRIPT_URL -OutFile $scriptPath -UseBasicParsing

        # Prepend env vars and append pause directly into the temp file
        # so we can use -File (most reliable parsing mode)
        $prefix = ""
        if ($env:LRC_API_KEY) { $prefix += "`$env:LRC_API_KEY = '$($env:LRC_API_KEY)'`r`n" }
        if ($env:LRC_API_URL) { $prefix += "`$env:LRC_API_URL = '$($env:LRC_API_URL)'`r`n" }
        $suffix = "`r`nWrite-Host ''`r`nWrite-Host 'Press any key to close...' -ForegroundColor Cyan`r`n`$null = `$Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown')`r`n"
        $scriptContent = [System.IO.File]::ReadAllText($scriptPath)
        [System.IO.File]::WriteAllText($scriptPath, $prefix + $scriptContent + $suffix)

        $p = Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $scriptPath `
            -Verb RunAs -Wait -PassThru -ErrorAction Stop
        Remove-Item $scriptPath -ErrorAction SilentlyContinue
        exit $p.ExitCode
    } catch {
        Write-Host "$FAIL Could not elevate to administrator." -ForegroundColor Red
        Print-ElevationHelp
        exit 1
    }
}

# --- From here on, we are running as administrator ---

# Require git to be present; we also install lrc alongside the git binary
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Host "Error: git is not installed. Please install git and retry." -ForegroundColor Red
    exit 1
}
$GIT_BIN = (Get-Command git).Source
$GIT_DIR = Split-Path -Parent $GIT_BIN

# B2 read-only credentials (hardcoded)
$B2_KEY_ID = "00536b4c5851afd0000000006"
$B2_APP_KEY = "K005DV+hNk6/fdQr8oXHmRsdo8U2YAU"
$B2_BUCKET_NAME = "hexmos"
$B2_PREFIX = "lrc"

Write-Host "lrc Installer" -ForegroundColor Cyan
Write-Host "================" -ForegroundColor Cyan
Write-Host ""

# Detect architecture
$ARCH = $env:PROCESSOR_ARCHITECTURE
switch ($ARCH) {
    "AMD64" { $PLATFORM_ARCH = "amd64" }
    "ARM64" { $PLATFORM_ARCH = "arm64" }
    default {
        Write-Host "Error: Unsupported architecture: $ARCH" -ForegroundColor Red
        exit 1
    }
}

$PLATFORM = "windows-$PLATFORM_ARCH"
Write-Host "$OK Detected platform: $PLATFORM" -ForegroundColor Green

# Install to Program Files (we have admin)
$INSTALL_DIR = "$env:ProgramFiles\lrc"
Write-Host "$OK Running as administrator; will install to $INSTALL_DIR" -ForegroundColor Green

# Ensure install directory exists
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
}

$INSTALL_PATH = "$INSTALL_DIR\lrc.exe"

# Authorize with B2
Write-Host -NoNewline "Authorizing with Backblaze B2... "
$authString = "${B2_KEY_ID}:${B2_APP_KEY}"
$authBytes = [System.Text.Encoding]::UTF8.GetBytes($authString)
$authBase64 = [System.Convert]::ToBase64String($authBytes)

try {
    $authResponse = Invoke-RestMethod -Uri "https://api.backblazeb2.com/b2api/v2/b2_authorize_account" `
        -Method Get `
        -Headers @{ "Authorization" = "Basic $authBase64" } `
        -UseBasicParsing
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Failed to authorize with B2" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

$AUTH_TOKEN = $authResponse.authorizationToken
$API_URL = $authResponse.apiUrl
$DOWNLOAD_URL = $authResponse.downloadUrl

# Create a WebSession to carry the B2 auth token.
# Some Windows/.NET versions reject raw B2 tokens in -Headers because they
# don't match a standard HTTP auth scheme (Bearer/Basic). WebRequestSession
# uses WebHeaderCollection which skips that validation.
$b2Session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$b2Session.Headers.Add("Authorization", $AUTH_TOKEN)

# List files in the lrc/ folder to find versions
Write-Host -NoNewline "Finding latest version... "
try {
    $listBody = @{
        bucketId = "33d6ab74ac456875919a0f1d"
        startFileName = "$B2_PREFIX/"
        prefix = "$B2_PREFIX/"
        maxFileCount = 10000
    } | ConvertTo-Json

    $listResponse = Invoke-RestMethod -Uri "$API_URL/b2api/v2/b2_list_file_names" `
        -Method Post `
        -WebSession $b2Session `
        -ContentType "application/json" `
        -Body $listBody `
        -UseBasicParsing
} catch {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Failed to list files from B2" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Extract versions that have a binary for our platform
# Only consider versions where lrc/<version>/<platform>/lrc.exe exists
# Use proper semantic version sorting (not lexicographic)
$versions = $listResponse.files |
    Where-Object { $_.fileName -match "^$B2_PREFIX/v[0-9]+\.[0-9]+\.[0-9]+/$PLATFORM/lrc\.exe$" } |
    ForEach-Object {
        if ($_.fileName -match "^$B2_PREFIX/(v[0-9]+\.[0-9]+\.[0-9]+)/") {
            $matches[1]
        }
    } |
    Select-Object -Unique |
    Sort-Object { [Version]($_ -replace '^v','') } -Descending

if (-not $versions -or ($versions | Measure-Object).Count -eq 0) {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: No versions found in $B2_BUCKET_NAME/$B2_PREFIX/" -ForegroundColor Red
    exit 1
}

# Handle both array and single-value returns
if ($versions -is [array]) {
    $LATEST_VERSION = $versions[0]
} else {
    $LATEST_VERSION = $versions
}
Write-Host "$OK Latest version: $LATEST_VERSION" -ForegroundColor Green

# Construct download URL
$BINARY_NAME = "lrc.exe"
$DOWNLOAD_PATH = "$B2_PREFIX/$LATEST_VERSION/$PLATFORM/$BINARY_NAME"
$FULL_URL = "$DOWNLOAD_URL/file/$B2_BUCKET_NAME/$DOWNLOAD_PATH"

Write-Host -NoNewline "Downloading lrc $LATEST_VERSION for $PLATFORM... "
$TMP_FILE = [System.IO.Path]::GetTempFileName()
try {
    Invoke-WebRequest -Uri $FULL_URL -OutFile $TMP_FILE -UseBasicParsing -WebSession $b2Session
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Failed to download" -ForegroundColor Red
    Write-Host "URL: $FULL_URL" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
    exit 1
}

# Check if file was downloaded
if (-not (Test-Path $TMP_FILE) -or (Get-Item $TMP_FILE).Length -eq 0) {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Downloaded file is empty or missing" -ForegroundColor Red
    Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
    exit 1
}

# Install binary
Write-Host -NoNewline "Installing to $INSTALL_PATH... "
try {
    Move-Item -Path $TMP_FILE -Destination $INSTALL_PATH -Force
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Failed to install to $INSTALL_PATH" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
    exit 1
}

# Copy to git directory as git-lrc.exe (git subcommand)
# Git discovers subcommands by looking for git-<name>.exe in PATH
$GIT_INSTALL_PATH = "$GIT_DIR\git-lrc.exe"
Write-Host -NoNewline "Installing to $GIT_INSTALL_PATH (git subcommand)... "
try {
    Copy-Item -Path $INSTALL_PATH -Destination $GIT_INSTALL_PATH -Force
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "$FAIL" -ForegroundColor Red
    Write-Host "Error: Failed to install git subcommand to $GIT_INSTALL_PATH" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Clean up any stale git-lrc.exe from user-local fallback dir (prevents version mismatch)
$userLocalFallback = "$env:LOCALAPPDATA\Programs\lrc\git-lrc.exe"
if ((Test-Path $userLocalFallback) -and ($userLocalFallback -ne $GIT_INSTALL_PATH)) {
    Remove-Item $userLocalFallback -Force -ErrorAction SilentlyContinue
}
$userLocalLrc = "$env:LOCALAPPDATA\Programs\lrc\lrc.exe"
if ((Test-Path $userLocalLrc) -and ($userLocalLrc -ne $INSTALL_PATH)) {
    Remove-Item $userLocalLrc -Force -ErrorAction SilentlyContinue
}

# Create config file if API key and URL are provided
if ($env:LRC_API_KEY -and $env:LRC_API_URL) {
    $CONFIG_FILE = "$env:USERPROFILE\.lrc.toml"

    # Check if config already exists
    if (Test-Path $CONFIG_FILE) {
        Write-Host "Note: Config file already exists at $CONFIG_FILE" -ForegroundColor Yellow

        # Read from console host even when piped
        $replaceConfig = "n"
        try {
            if ([Environment]::UserInteractive) {
                Write-Host -NoNewline "Replace existing config? [y/N]: "
                $replaceConfig = [Console]::ReadLine()
                if ([string]::IsNullOrWhiteSpace($replaceConfig)) {
                    $replaceConfig = "n"
                }
            }
        } catch {
            Write-Host "Replace existing config? [y/N]: n (defaulting to No)" -ForegroundColor Yellow
        }

        if ($replaceConfig -match '^[Yy]$') {
            Write-Host -NoNewline "Replacing config file at $CONFIG_FILE... "
            try {
                $configContent = @"
api_key = "$($env:LRC_API_KEY)"
api_url = "$($env:LRC_API_URL)"
"@
                Set-Content -Path $CONFIG_FILE -Value $configContent -NoNewline
                # Restrict config file to current user only (contains API key)
                $acl = Get-Acl $CONFIG_FILE
                $acl.SetAccessRuleProtection($true, $false)
                $acl.Access | ForEach-Object { $acl.RemoveAccessRule($_) } | Out-Null
                $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
                    [System.Security.Principal.WindowsIdentity]::GetCurrent().Name,
                    "FullControl", "Allow")
                $acl.AddAccessRule($rule)
                Set-Acl -Path $CONFIG_FILE -AclObject $acl
                Write-Host "$OK" -ForegroundColor Green
                Write-Host "Config file replaced with your API credentials" -ForegroundColor Green
            } catch {
                Write-Host "$FAIL" -ForegroundColor Red
                Write-Host "Warning: Failed to replace config file" -ForegroundColor Yellow
                Write-Host $_.Exception.Message -ForegroundColor Yellow
            }
        } else {
            Write-Host "Skipping config creation to preserve existing settings" -ForegroundColor Yellow
        }
    } else {
        Write-Host -NoNewline "Creating config file at $CONFIG_FILE... "
        try {
            $configContent = @"
api_key = "$($env:LRC_API_KEY)"
api_url = "$($env:LRC_API_URL)"
"@
            Set-Content -Path $CONFIG_FILE -Value $configContent -NoNewline
            # Restrict config file to current user only (contains API key)
            $acl = Get-Acl $CONFIG_FILE
            $acl.SetAccessRuleProtection($true, $false)
            $acl.Access | ForEach-Object { $acl.RemoveAccessRule($_) } | Out-Null
            $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
                [System.Security.Principal.WindowsIdentity]::GetCurrent().Name,
                "FullControl", "Allow")
            $acl.AddAccessRule($rule)
            Set-Acl -Path $CONFIG_FILE -AclObject $acl
            Write-Host "$OK" -ForegroundColor Green
            Write-Host "Config file created with your API credentials" -ForegroundColor Green
        } catch {
            Write-Host "$FAIL" -ForegroundColor Red
            Write-Host "Warning: Failed to create config file" -ForegroundColor Yellow
            Write-Host $_.Exception.Message -ForegroundColor Yellow
        }
    }
}

# Add to PATH if not already there (with deduplication)
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not $currentPath) { $currentPath = "" }
$normalizedInstallDir = $INSTALL_DIR.TrimEnd('\')
$pathEntries = $currentPath -split ';' | ForEach-Object { $_.TrimEnd('\') } | Where-Object { $_ -ne '' }
if ($normalizedInstallDir -notin $pathEntries) {
    Write-Host -NoNewline "Adding $INSTALL_DIR to PATH... "
    try {
        if ($currentPath -eq "") { $newPath = $INSTALL_DIR } else { $newPath = "$currentPath;$INSTALL_DIR" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        # Update current session PATH
        $env:Path = "$env:Path;$INSTALL_DIR"
        Write-Host "$OK" -ForegroundColor Green
        Write-Host ""
        Write-Host "Note: You may need to restart your terminal for PATH changes to take effect" -ForegroundColor Yellow
    } catch {
        Write-Host "$FAIL" -ForegroundColor Red
        Write-Host "Warning: Could not add to PATH automatically" -ForegroundColor Yellow
        Write-Host "Please add $INSTALL_DIR to your PATH manually" -ForegroundColor Yellow
    }
}

# Install global hooks via lrc
Write-Host -NoNewline "Running 'lrc hooks install' to set up global hooks... "
try {
    & $INSTALL_PATH hooks install 2>&1 | Out-Null
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "(warning)" -ForegroundColor Yellow
    Write-Host "Warning: Failed to run 'lrc hooks install'. You may need to run it manually." -ForegroundColor Yellow
}

# Track CLI installation if API key and URL are available
if ($env:LRC_API_KEY -and $env:LRC_API_URL) {
    Write-Host -NoNewline "Notifying LiveReview about CLI installation... "
    try {
        $headers = @{
            "X-API-Key" = $env:LRC_API_KEY
            "Content-Type" = "application/json"
        }
        $trackUrl = "$($env:LRC_API_URL)/api/v1/diff-review/cli-used"
        Invoke-RestMethod -Uri $trackUrl -Method Post -Headers $headers -UseBasicParsing | Out-Null
        Write-Host "$OK" -ForegroundColor Green
    } catch {
        Write-Host "(skipped)" -ForegroundColor Yellow
    }
}

# Verify installation
Write-Host ""
Write-Host "$OK Installation complete!" -ForegroundColor Green
Write-Host ""
try { & $INSTALL_PATH version } catch { }
Write-Host ""

# Verify version consistency between lrc and git-lrc
$lrcVer = (& $INSTALL_PATH version 2>&1 | Select-String "v[0-9]+\.[0-9]+\.[0-9]+" | ForEach-Object { $_.Matches[0].Value }) 2>$null
$gitLrcVer = (& $GIT_INSTALL_PATH version 2>&1 | Select-String "v[0-9]+\.[0-9]+\.[0-9]+" | ForEach-Object { $_.Matches[0].Value }) 2>$null
if ($lrcVer -and $gitLrcVer -and ($lrcVer -ne $gitLrcVer)) {
    Write-Host "WARNING: Version mismatch! lrc=$lrcVer but git-lrc=$gitLrcVer" -ForegroundColor Red
} elseif ($lrcVer -and $gitLrcVer) {
    Write-Host "$OK lrc and git-lrc both at $lrcVer" -ForegroundColor Green
}

Write-Host ""
Write-Host "Run 'lrc --help' to get started"
