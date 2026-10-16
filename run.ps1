#requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('mygo', 'wails', 'mygo-native')]
    [string]$Framework = 'mygo',
    [switch]$RebuildFrontend,
    [switch]$Check,
    [switch]$NoPause,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($Help) {
    Write-Output 'Usage: .\run.ps1 [-Framework mygo|wails|mygo-native] [-RebuildFrontend] [-Check] [-NoPause]'
    Write-Output 'Default: mygo. Uses go run; reuses unchanged frontend output.'
    Write-Output '-Check prepares dependencies and compiles without opening the app. -NoPause skips the error prompt.'
    exit 0
}

$frontendDirectory = Join-Path $PSScriptRoot 'frontend'

function Test-NewerInput([string]$Path, [datetime]$After) {
    if (-not (Test-Path -LiteralPath $Path)) { return $false }
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.Name -match '(?i)trash') { return $false }
    if ($item.LastWriteTimeUtc -gt $After) { return $true }
    if ($item.PSIsContainer -and -not ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        foreach ($child in Get-ChildItem -LiteralPath $Path -Force) {
            if ($child.Name -notmatch '(?i)trash' -and (Test-NewerInput $child.FullName $After)) {
                return $true
            }
        }
    }
    return $false
}

function Test-FrontendChanged {
    $entry = Join-Path $frontendDirectory 'dist/index.html'
    if ($RebuildFrontend -or -not (Test-Path -LiteralPath $entry -PathType Leaf)) { return $true }
    $output = Get-Item -LiteralPath $entry
    if ($output.Length -eq 0 -or -not (Test-Path -LiteralPath (Join-Path $frontendDirectory 'dist/assets') -PathType Container)) {
        return $true
    }
    # Directory timestamps also catch deleted source files.
    foreach ($inputPath in @('frontend/src', 'frontend/public', 'frontend/index.html',
        'frontend/package.json', 'frontend/package-lock.json', 'frontend/tsconfig.json',
        'frontend/vite.config.ts', 'resources/locales', 'resources/skins.json',
        'resources/branding/yourun-rabbit-paths.json')) {
        if (Test-NewerInput (Join-Path $PSScriptRoot $inputPath) $output.LastWriteTimeUtc) { return $true }
    }
    return $false
}

$exitCode = 1
$failed = $false
$locationPushed = $false
$transcribing = $false
$logPath = $null
$checkExecutable = $null
$savedPath = $env:Path
$handoffNames = @('COMMAND_MANAGER_UPDATE_PARENT', 'COMMAND_MANAGER_UPDATE_TOKEN', 'COMMAND_MANAGER_UPDATE_REDIRECT')
$savedHandoff = @{}
foreach ($name in $handoffNames) {
    $savedHandoff[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

try {
    $logRoot = [Environment]::GetFolderPath('LocalApplicationData')
    if ([string]::IsNullOrEmpty($logRoot)) { $logRoot = [IO.Path]::GetTempPath() }
    $logDirectory = Join-Path $logRoot 'command-manager/launch-logs'
    New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
    $logPath = Join-Path $logDirectory ('run-{0}-{1}.log' -f (Get-Date -Format 'yyyyMMdd-HHmmss-fff'), $PID)
    Start-Transcript -Path $logPath -Force | Out-Null
    $transcribing = $true

    if ($env:OS -eq 'Windows_NT') {
        # Explorer can retain PATH from before Go/Node were installed.
        # Keep the caller's entries first and append the current persisted paths.
        $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $persistedPath = [Environment]::ExpandEnvironmentVariables($machinePath + ';' + $userPath)
        $env:Path = $savedPath + ';' + $persistedPath
    }
    $goCommand = (Get-Command go -CommandType Application -ErrorAction Stop).Source
    Write-Host ('Go: ' + $goCommand)
    Push-Location -LiteralPath $PSScriptRoot
    $locationPushed = $true
    if ($Framework -ne 'mygo-native' -and (Test-FrontendChanged)) {
        $null = Get-Command node -CommandType Application -ErrorAction Stop
        $npmName = if ($env:OS -eq 'Windows_NT') { 'npm.cmd' } else { 'npm' }
        $npmCommand = (Get-Command $npmName -CommandType Application -ErrorAction Stop).Source
        Set-Location -LiteralPath $frontendDirectory
        $installed = Join-Path $frontendDirectory 'node_modules/.package-lock.json'
        $install = -not (Test-Path -LiteralPath $installed -PathType Leaf)
        if (-not $install) {
            $installedAt = (Get-Item -LiteralPath $installed).LastWriteTimeUtc
            $install = (Test-NewerInput (Join-Path $frontendDirectory 'package-lock.json') $installedAt) -or
                (Test-NewerInput (Join-Path $frontendDirectory 'package.json') $installedAt)
        }
        if ($install) {
            & $npmCommand ci --ignore-scripts --no-audit --no-fund
            if ($LASTEXITCODE -ne 0) {
                $exitCode = $LASTEXITCODE
                throw "npm ci failed (exit $exitCode)."
            }
        }
        & $npmCommand run build
        if ($LASTEXITCODE -ne 0) {
            $exitCode = $LASTEXITCODE
            throw "Frontend build failed (exit $exitCode)."
        }
    }

    foreach ($name in $handoffNames) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
    Set-Location -LiteralPath (Join-Path $PSScriptRoot $Framework)
    if ($Check) {
        $checkExecutable = Join-Path ([IO.Path]::GetTempPath()) ('yourun-launch-check-' + [guid]::NewGuid().ToString('N') + '.exe')
        Write-Host ('Checking ' + $Framework + '...')
        & $goCommand build -o $checkExecutable .
    }
    else {
        Write-Host ('Starting ' + $Framework + '...')
        & $goCommand run .
    }
    if ($LASTEXITCODE -ne 0) {
        $exitCode = $LASTEXITCODE
        throw "Go command failed (exit $exitCode)."
    }
    $exitCode = 0
}
catch {
    $failed = $true
    Write-Host ('Launch failed: ' + $_.Exception.Message) -ForegroundColor Red
    Write-Host $_.InvocationInfo.PositionMessage -ForegroundColor Red
    if ($logPath) { Write-Host ('Log: ' + $logPath) }
}
finally {
    if ($locationPushed) { Pop-Location }
    $env:Path = $savedPath
    foreach ($name in $handoffNames) { [Environment]::SetEnvironmentVariable($name, $savedHandoff[$name], 'Process') }
    if ($checkExecutable -and (Test-Path -LiteralPath $checkExecutable -PathType Leaf)) {
        Remove-Item -LiteralPath $checkExecutable -Force -ErrorAction SilentlyContinue
    }
    if ($transcribing) { Stop-Transcript | Out-Null }
}
if ($failed -and -not $NoPause -and [Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
    Read-Host 'Press Enter to close' | Out-Null
}
exit $exitCode
