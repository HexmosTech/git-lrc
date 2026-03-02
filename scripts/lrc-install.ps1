# lrc installer for Windows PowerShell
# Usage: iwr -useb https://your-domain/lrc-install.ps1 | iex
#   or:  Invoke-WebRequest -Uri https://your-domain/lrc-install.ps1 -UseBasicParsing | Invoke-Expression
#
# Install model (concise):
# - Default: per-user install to %LOCALAPPDATA%\Programs\lrc (user-writable install dir; no admin needed).
# - Migration: if legacy admin binaries exist (Program Files or Git bin), elevate once to delete them, then continue non-admin.
# - Discovery: place lrc.exe + git-lrc.exe in the user install dir, add WindowsApps (%LOCALAPPDATA%\Microsoft\WindowsApps) cmd/exe shims, prepend PATH in-session; Git bin copy only when writable (no forced elevation).
# - No shell restart required: PATH and PATHEXT adjusted in-session; shims + PATH prep give immediate git subcommand resolution.
# - PATH persistence: user PATH is updated to include %LOCALAPPDATA%\Programs\lrc; current session PATH is also prepended so it works right away.
# - Logging: migration cleanup logs to %TEMP%\lrc-cleanup.log when elevation is used.

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

# Detect admin status once; elevation is only needed for legacy cleanup, not for fresh installs
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

function Test-UserWritable {
    param([string]$directory)
    if (-not (Test-Path $directory)) { return $false }
    $testPath = Join-Path $directory (".__lrc_write_test_" + [System.IO.Path]::GetRandomFileName())
    try {
        New-Item -ItemType File -Path $testPath -Force -ErrorAction Stop | Out-Null
        Remove-Item -Path $testPath -Force -ErrorAction SilentlyContinue
        return $true
    } catch {
        return $false
    }
}

# Require git to be present; we install a git subcommand alongside PATH
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Host "Error: git is not installed. Please install git and retry." -ForegroundColor Red
    exit 1
}
$GIT_BIN = (Get-Command git).Source
$GIT_DIR = Split-Path -Parent $GIT_BIN

# Preferred install targets (user-writable)
$INSTALL_DIR = "$env:LOCALAPPDATA\Programs\lrc"
$INSTALL_PATH = "$INSTALL_DIR\lrc.exe"
$GIT_INSTALL_PATH = "$INSTALL_DIR\git-lrc.exe"  # git discovers subcommands on PATH

# Legacy admin-scope locations we want to remove (one-time migration)
$ADMIN_INSTALL_DIR = "$env:ProgramFiles\lrc"
$ADMIN_INSTALL_PATH = "$ADMIN_INSTALL_DIR\lrc.exe"
$GIT_DIR_GIT_LRC_PATH = "$GIT_DIR\git-lrc.exe"

$needsAdminCleanup = $false
$cleanupTargets = @()
if (Test-Path $ADMIN_INSTALL_PATH) {
    $needsAdminCleanup = $true
    $cleanupTargets += $ADMIN_INSTALL_PATH
}
if (Test-Path $GIT_DIR_GIT_LRC_PATH) {
    if (Test-UserWritable -directory $GIT_DIR) {
        try { Remove-Item -Path $GIT_DIR_GIT_LRC_PATH -Force -ErrorAction SilentlyContinue } catch { }
    } else {
        $needsAdminCleanup = $true
        $cleanupTargets += $GIT_DIR_GIT_LRC_PATH
    }
}

if ($needsAdminCleanup) {
    if ($isAdmin) {
        Write-Host "Removing legacy admin-installed binaries..." -ForegroundColor Yellow
        foreach ($p in $cleanupTargets) {
            if (Test-Path $p) {
                try { Remove-Item -Path $p -Force -ErrorAction SilentlyContinue; Write-Host "Removed $p" -ForegroundColor Green } catch { }
            }
        }
    } else {
        function Invoke-ElevatedCleanup {
            param([string[]]$paths)
            $cleanupScript = Join-Path $env:TEMP "lrc-cleanup-elevated.ps1"
            $targetsFile = Join-Path $env:TEMP "lrc-cleanup-targets.txt"
            $logPath = Join-Path $env:TEMP "lrc-cleanup.log"

            # Write targets to a file to preserve spaces
            Set-Content -Path $targetsFile -Value ($paths -join "`n") -Encoding UTF8 -NoNewline

            $scriptBody = @"
param([string]`$targetsFile, [string]`$logPath)
function Log([string]`$msg) { `$msg | Out-File -FilePath `$logPath -Encoding UTF8 -Append }
Log "-----"
Log "Cleanup start: $(Get-Date -Format o)"
Log "Running as: $([Security.Principal.WindowsIdentity]::GetCurrent().Name) (Admin=$([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))"
`$paths = Get-Content -Path `$targetsFile -ErrorAction SilentlyContinue
foreach (`$p in `$paths) {
    Log "Target: `$p"
    Log "Exists(before): $(Test-Path `$p)"
    try {
        if (Test-Path `$p) { Remove-Item -Path `$p -Force -ErrorAction Stop; Log "Removed `$p" }
    } catch {
        Log "Error removing `$p : $($_.Exception.Message)"
    }
    Log "Exists(after): $(Test-Path `$p)"
}
Log "Cleanup end: $(Get-Date -Format o)"
"@ 

            Set-Content -Path $cleanupScript -Value $scriptBody -Encoding UTF8 -NoNewline
            $cleanupArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $cleanupScript, "-targetsFile", $targetsFile, "-logPath", $logPath)
            $p = Start-Process powershell -ArgumentList $cleanupArgs -Verb RunAs -Wait -PassThru -ErrorAction Stop
            try { Remove-Item -Path $cleanupScript -Force -ErrorAction SilentlyContinue } catch { }
            try { Remove-Item -Path $targetsFile -Force -ErrorAction SilentlyContinue } catch { }
            return @{ ExitCode = $p.ExitCode; LogPath = $logPath }
        }

        Write-Host "Found legacy admin-installed binaries. Elevating once to remove them before reinstalling to a user-writable location..." -ForegroundColor Yellow
        try {
            $cleanupResult = Invoke-ElevatedCleanup -paths $cleanupTargets
            if ($cleanupResult.ExitCode -ne 0) {
                Write-Host "$FAIL Could not remove legacy admin binaries (exit $($cleanupResult.ExitCode))." -ForegroundColor Red
                Write-Host "Cleanup log: $($cleanupResult.LogPath)" -ForegroundColor Yellow
                try { Get-Content -Path $cleanupResult.LogPath -ErrorAction Stop | Select-Object -Last 200 | ForEach-Object { Write-Host $_ } } catch { }
                Print-ElevationHelp
                Read-Host "Press Enter to exit"
                exit 1
            }
        } catch {
            Write-Host "$FAIL Could not remove legacy admin binaries." -ForegroundColor Red
            Print-ElevationHelp
            Read-Host "Press Enter to exit"
            exit 1
        }
    }

    $remaining = @($cleanupTargets | Where-Object { Test-Path $_ })
    if ($remaining.Count -gt 0) {
        Write-Host "$FAIL Legacy admin binaries still present: $($remaining -join ', ')" -ForegroundColor Red
        Print-ElevationHelp
        Write-Host "Check cleanup log at $env:TEMP\lrc-cleanup.log" -ForegroundColor Yellow
        try { Get-Content -Path (Join-Path $env:TEMP "lrc-cleanup.log") -ErrorAction Stop | Select-Object -Last 200 | ForEach-Object { Write-Host $_ } } catch { }
        Read-Host "Press Enter to exit"
        exit 1
    }

    Write-Host "$OK Legacy admin binaries removed." -ForegroundColor Green
}

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

# Ensure install directory exists (user-writable by default)
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
}

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

# Create git-lrc.exe alongside lrc.exe so Git can discover it via PATH
Write-Host -NoNewline "Creating $GIT_INSTALL_PATH (git subcommand)... "
try {
    Copy-Item -Path $INSTALL_PATH -Destination $GIT_INSTALL_PATH -Force
    Write-Host "$OK" -ForegroundColor Green
} catch {
    Write-Host "(warning)" -ForegroundColor Yellow
    Write-Host "Warning: Failed to create $GIT_INSTALL_PATH; git lrc may not resolve until PATH picks up lrc.exe." -ForegroundColor Yellow
}

# Optionally copy git-lrc into the Git bin directory when writable (improves discovery)
$gitBinWritable = Test-UserWritable -directory $GIT_DIR
if ($gitBinWritable) {
    $gitBinTarget = Join-Path $GIT_DIR "git-lrc.exe"
    Write-Host -NoNewline "Copying git-lrc.exe to Git bin ($gitBinTarget)... "
    try {
        Copy-Item -Path $GIT_INSTALL_PATH -Destination $gitBinTarget -Force
        Write-Host "$OK" -ForegroundColor Green
    } catch {
        Write-Host "(warning)" -ForegroundColor Yellow
        Write-Host "Warning: Could not copy git-lrc.exe into Git bin; PATH-based discovery will be used." -ForegroundColor Yellow
    }
} else {
    Write-Host "Git bin not writable; relying on PATH-based discovery and WindowsApps shim." -ForegroundColor Yellow
}

# Create a cmd shim as an additional fallback for git subcommand discovery
$gitLrcCmdShim = Join-Path $INSTALL_DIR "git-lrc.cmd"
try {
    $shimContent = '@echo off`r`n"%~dp0lrc.exe" %*'
    Set-Content -Path $gitLrcCmdShim -Value $shimContent -NoNewline -Encoding ASCII
} catch { }

# Create user WindowsApps shims to ensure immediate PATH resolution without shell restart
$windowsApps = Join-Path $env:LocalAppData "Microsoft\WindowsApps"
try {
    if (-not (Test-Path $windowsApps)) { New-Item -ItemType Directory -Path $windowsApps -Force | Out-Null }
    $lrcShimPath = Join-Path $windowsApps "lrc.cmd"
    $gitLrcShimPath = Join-Path $windowsApps "git-lrc.cmd"
    $shimBody = "@echo off`r`n`"$INSTALL_PATH`" %*"
    $gitShimBody = "@echo off`r`n`"$GIT_INSTALL_PATH`" %*"
    Set-Content -Path $lrcShimPath -Value $shimBody -Encoding ASCII -NoNewline
    Set-Content -Path $gitLrcShimPath -Value $gitShimBody -Encoding ASCII -NoNewline
    # Also drop git-lrc.exe into WindowsApps to help git subcommand discovery
    $windowsAppsGitExe = Join-Path $windowsApps "git-lrc.exe"
    Copy-Item -Path $GIT_INSTALL_PATH -Destination $windowsAppsGitExe -Force -ErrorAction SilentlyContinue
    # Prepend WindowsApps for this session to ensure Git sees the exe immediately
    if (-not ($env:Path.Split(';') -contains $windowsApps)) {
        $env:Path = "$windowsApps;$env:Path"
    }
} catch { }

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
        Write-Host "$OK" -ForegroundColor Green
        Write-Host ""
        Write-Host "Note: You may need to restart your terminal for PATH changes to take effect" -ForegroundColor Yellow
    } catch {
        Write-Host "$FAIL" -ForegroundColor Red
        Write-Host "Warning: Could not add to PATH automatically" -ForegroundColor Yellow
        Write-Host "Please add $INSTALL_DIR to your PATH manually" -ForegroundColor Yellow
    }
}
# Always prepend install dir to current session PATH to win over any lingering entries
$env:Path = "$INSTALL_DIR;$env:Path"
# Ensure PATHEXT contains .CMD so git picks up the cmd shim
if (-not ($env:PATHEXT -match "\.CMD(;|$)")) { $env:PATHEXT = "$env:PATHEXT;.CMD" }

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

# If admin-scope binaries somehow remain, warn
$adminLeftovers = @()
if (Test-Path $ADMIN_INSTALL_PATH) { $adminLeftovers += $ADMIN_INSTALL_PATH }
if ((Test-Path $GIT_DIR_GIT_LRC_PATH) -and (-not (Test-UserWritable -directory $GIT_DIR))) { $adminLeftovers += $GIT_DIR_GIT_LRC_PATH }
if ($adminLeftovers.Count -gt 0) {
    Write-Host "(warning) Admin-scope binaries still exist and may shadow the user install: $($adminLeftovers -join ', ')" -ForegroundColor Yellow
    Write-Host "Please remove them manually or rerun this installer as Administrator to clean them." -ForegroundColor Yellow
}

# Verify git resolves the subcommand; if not, attempt a retry with the shim/copy
function Test-GitLrc {
    try {
        $out = (& git lrc version 2>&1)
        $ver = ($out | Select-String "v[0-9]+\.[0-9]+\.[0-9]+" | ForEach-Object { $_.Matches[0].Value }) 2>$null
        if ($ver) { return @{ success = $true; version = $ver; output = $out } }
        return @{ success = $false; version = $null; output = $out }
    } catch {
        return @{ success = $false; version = $null; output = $_.Exception.Message }
    }
}

$gitCheck = Test-GitLrc
if (-not $gitCheck.success) {
    # Retry once after ensuring PATH is prefixed
    $env:Path = "$INSTALL_DIR;$env:Path"
    # If Git bin is writable, ensure the copy exists
    if ($gitBinWritable -and -not (Test-Path (Join-Path $GIT_DIR "git-lrc.exe"))) {
        try { Copy-Item -Path $GIT_INSTALL_PATH -Destination (Join-Path $GIT_DIR "git-lrc.exe") -Force } catch { }
    }
    $gitCheck = Test-GitLrc
}

if ($gitCheck.success) {
    Write-Host "$OK git lrc resolves (version $($gitCheck.version))" -ForegroundColor Green
} else {
    Write-Host "(warning) git lrc did not resolve; git output: $($gitCheck.output)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Run 'lrc --help' to get started"
