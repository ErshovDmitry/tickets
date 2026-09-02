# smoke-windows.ps1 - Windows smoke test for the Go ticket binary (ticket.exe).
#
# Purpose:
#   Verify the cross-compiled Windows binary (dist/ticket.exe, see
#   AGENTS_ARCHITECTURE.md sections 5, 7, 8) on a Windows host (internal Windows test host):
#     1. new   -> creates T-0001-open.md and prints its path
#     2. list  -> shows the ticket
#     3. show  -> prints the ticket body
#     4. set   -> renames to T-0001-wip.md, updates status line + journal
#     5. exe-relative resolution: run by absolute path from a foreign CWD
#     6. TICKETS_DIR env override
#     7. parallel new x5 -> 5 unique sequential numbers (OS lock)
#   Runs entirely inside a temp sandbox; repo and user data are untouched.
#
# Usage (manually over SSH on the Windows host):
#   powershell -ExecutionPolicy Bypass -File scripts\smoke-windows.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\smoke-windows.ps1 -TicketExe C:\path\to\ticket.exe
#   Exit code: 0 = all steps passed, 1 = one or more steps failed (or exe missing).
#
# NOTE: this file MUST stay pure 7-bit ASCII. Windows PowerShell 5.1 parses a
# BOM-less .ps1 in the system ANSI code page, so any non-ASCII byte (Cyrillic
# text, smart quotes, dashes) would corrupt parsing. All messages are English.

param(
    [string]$TicketExe = $(if ($PSScriptRoot) { Join-Path $PSScriptRoot '..\dist\ticket.exe' } else { '..\dist\ticket.exe' })
)

$ErrorActionPreference = 'Stop'

$script:passes = 0
$script:fails  = 0

function Write-Pass {
    param([string]$Message)
    $script:passes++
    Write-Host "[PASS] $Message"
}

function Write-Fail {
    param([string]$Message)
    $script:fails++
    Write-Host "[FAIL] $Message"
}

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

# Invoke the ticket exe; return exit code + stdout as one string.
# NOTE: do not use Out-String here - it wraps long lines to console width,
# which would corrupt exact-match assertions on printed paths.
function Invoke-Ticket {
    param(
        [Parameter(Mandatory=$true)][string]$ExePath,
        [string[]]$Arguments = @(),
        [string]$WorkingDirectory = ''
    )
    $prevCwd = (Get-Location).Path
    if ($WorkingDirectory) { Set-Location -LiteralPath $WorkingDirectory }
    try {
        $lines = @(& $ExePath @Arguments)
        $code = $LASTEXITCODE
    } finally {
        Set-Location -LiteralPath $prevCwd
    }
    $stdout = (@($lines | ForEach-Object { [string]$_ }) -join "`n").TrimEnd()
    return [pscustomobject]@{ ExitCode = $code; Stdout = $stdout }
}

# Preflight: binary must exist.
if (-not (Test-Path -LiteralPath $TicketExe -PathType Leaf)) {
    Write-Host "[FAIL] ticket.exe not found: $TicketExe"
    Write-Host "       build it first (dev build, no version stamp): GOOS=windows GOARCH=amd64 go build -o dist/ticket.exe ./cmd/ticket"
    Write-Host "       with version: `$ver = (git describe --tags --always 2>`$null) -replace '^v',''; if (-not `$ver) { `$ver = 'dev' }; go build -ldflags `"-X ticket/internal/cli.version=`$ver`" -o dist/ticket.exe ./cmd/ticket"
    Write-Host "SMOKE RESULT: FAIL (exe missing)"
    exit 1
}
Write-Host "Windows smoke test for ticket.exe"
Write-Host "Binary: $TicketExe"

# Ticket files use Russian UI text (bash-reference compatible). Build the only
# Cyrillic fragment we assert on from Unicode code points so that this script
# file stays pure ASCII: the body line checked below is "- <RU 'status'>: wip".
$statLabel = -join [char[]](0x0421, 0x0442, 0x0430, 0x0442, 0x0443, 0x0441)  # RU word for "status"
$statWip = '- ' + $statLabel + ': wip'

# Clean TICKETS_DIR baseline for the whole run; restore the original at exit
# (otherwise a pre-existing override would send smoke tickets elsewhere).
$prevTicketsDir = $env:TICKETS_DIR
if ($prevTicketsDir) {
    Remove-Item Env:TICKETS_DIR -ErrorAction SilentlyContinue
    Write-Host "[WARN] pre-existing TICKETS_DIR cleared for the run (restored at exit): $prevTicketsDir"
}

$cleanup = @()
$sb = $null
try {
    # Sandbox: <temp>\ticket-smoke-<guid>\tickets\bin\ticket.exe
    try {
        $sb = Join-Path $env:TEMP ('ticket-smoke-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $sb 'tickets\bin') -Force | Out-Null
        $cleanup += $sb
        Copy-Item -LiteralPath $TicketExe -Destination (Join-Path $sb 'tickets\bin\ticket.exe')
    } catch {
        Write-Host ("[FAIL] sandbox setup: " + $_.Exception.Message)
        Write-Host "SMOKE RESULT: FAIL (sandbox setup)"
        exit 1
    }
    $exe = Join-Path $sb 'tickets\bin\ticket.exe'
    $ticketsDir = Join-Path $sb 'tickets'
    Write-Host "Sandbox: $sb"
    Write-Host ''

    # ---- Step 1: new ----
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('new', 'smoke test ticket', '-t', 'OPS', '-p', 'low', '-d', 'smoke details') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        $expected1 = Join-Path $ticketsDir 'T-0001-open.md'
        Assert-True (Test-Path -LiteralPath $expected1) ("file not created: " + $expected1)
        $printed = [System.IO.Path]::GetFullPath($r.Stdout.Trim())
        Assert-True ([string]::Equals($printed, $expected1, [StringComparison]::OrdinalIgnoreCase)) ("printed path mismatch: '" + $printed + "'")
        Write-Pass 'new: T-0001-open.md created, printed path matches'
    } catch {
        Write-Fail ('new: ' + $_.Exception.Message)
    }

    # ---- Step 2: list ----
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('list') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match 'T-0001') ("output does not contain T-0001: '" + $r.Stdout + "'")
        Assert-True ($r.Stdout -match 'open') 'output does not contain status open'
        Write-Pass 'list: shows T-0001 as open'
    } catch {
        Write-Fail ('list: ' + $_.Exception.Message)
    }

    # ---- Step 3: show ----
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('show', '1') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match 'smoke test ticket') 'output does not contain ticket title'
        Write-Pass 'show: body contains ticket title'
    } catch {
        Write-Fail ('show: ' + $_.Exception.Message)
    }

    # ---- Step 4: set 1 wip ----
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('set', '1', 'wip', 'smoke transition') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        $wipFile = Join-Path $ticketsDir 'T-0001-wip.md'
        Assert-True (Test-Path -LiteralPath $wipFile) ("renamed file not found: " + $wipFile)
        $content = Get-Content -LiteralPath $wipFile -Raw -Encoding UTF8
        Assert-True ($null -ne $content -and $content.Contains($statWip)) 'status line not updated in ticket body'
        Assert-True ($content.Contains('smoke transition')) 'journal line with comment not found'
        Write-Pass 'set: renamed to T-0001-wip.md, status line and journal updated'
    } catch {
        Write-Fail ('set: ' + $_.Exception.Message)
    }

    # ---- Step 5: foreign CWD, absolute exe path (exe-relative resolution) ----
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('list') -WorkingDirectory $env:TEMP
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match 'T-0001') 'ticket not visible from a foreign CWD'
        Write-Pass 'foreign CWD: exe-relative tickets dir resolution works'
    } catch {
        Write-Fail ('foreign CWD: ' + $_.Exception.Message)
    }

    # ---- Step 6: TICKETS_DIR override ----
    try {
        $envT = Join-Path $env:TEMP ('ticket-smoke-env-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $envT -Force | Out-Null
        $cleanup += $envT
        $env:TICKETS_DIR = $envT
        $r = Invoke-Ticket -ExePath $exe -Arguments @('new', 'smoke env ticket', '-t', 'TD', '-p', 'normal', '-d', 'env override') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("exit code " + $r.ExitCode + " (expected 0)")
        $envFile = Join-Path $envT 'T-0001-open.md'
        Assert-True (Test-Path -LiteralPath $envFile) ("file not created in TICKETS_DIR: " + $envFile)
        Write-Pass 'TICKETS_DIR: new ticket created in override directory'
    } catch {
        Write-Fail ('TICKETS_DIR: ' + $_.Exception.Message)
    } finally {
        Remove-Item Env:TICKETS_DIR -ErrorAction SilentlyContinue
    }

    # ---- Step 7: parallel new x5 (atomic numbering under OS lock) ----
    try {
        # Direct .NET Process APIs: PS 5.1 Start-Process -PassThru does not
        # reliably expose .ExitCode (observed $null even after the timed wait
        # followed by the parameterless WaitForExit() repair), while a Process
        # started directly from ProcessStartInfo keeps its own handle and
        # reports ExitCode reliably. On .NET Framework UseShellExecute defaults
        # to true - it must be false for stream redirection to work.
        $procs = @()
        for ($i = 1; $i -le 5; $i++) {
            $psi = New-Object System.Diagnostics.ProcessStartInfo
            $psi.FileName = $exe
            $psi.Arguments = 'new "smoke-parallel-' + $i + '" -t ENH -p low -d "parallel-smoke-' + $i + '"'
            $psi.WorkingDirectory = $sb
            $psi.UseShellExecute = $false
            $psi.CreateNoWindow = $true
            $psi.RedirectStandardOutput = $true
            $psi.RedirectStandardError = $true
            # No waiting inside the launch loop: all five processes run at once
            # (a wait here would serialize them and defeat the race); we wait
            # on each below (30 s timeout per process).
            $procs += [System.Diagnostics.Process]::Start($psi)
        }
        $done = @()
        foreach ($p in $procs) {
            if (-not $p.WaitForExit(30000)) {
                Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
                throw 'process did not exit within 30 s'
            }
            # The process has exited here: pipes are flushed and closed, so the
            # blocking drains below return immediately (stdout is one short
            # line, far below the pipe buffer - no deadlock). Drain both streams
            # to keep the console clean.
            $done += [pscustomobject]@{ Code = $p.ExitCode; Stdout = $p.StandardOutput.ReadToEnd().Trim() }
            $null = $p.StandardError.ReadToEnd()
        }
        $paths = @()
        foreach ($d in $done) {
            Assert-True ($d.Code -eq 0) ("one process exited with code " + $d.Code)
            Assert-True ($d.Stdout.Length -gt 0) 'process printed nothing'
            $paths += $d.Stdout
        }
        $nums = @()
        foreach ($pth in $paths) {
            $name = [System.IO.Path]::GetFileName($pth)
            if ($name -match '^T-(\d{4})-') {
                $nums += $Matches[1]
                $full = Join-Path $ticketsDir $name
                Assert-True (Test-Path -LiteralPath $full) ("file missing: " + $full)
            } else {
                throw ("unexpected stdout, expected ticket path: '" + $pth + "'")
            }
        }
        $uniq = @($nums | Select-Object -Unique)
        Assert-True ($uniq.Count -eq 5) ("expected 5 unique numbers, got " + $uniq.Count + " (" + ($nums -join ',') + ")")
        Write-Pass 'parallel new x5: all exit 0, 5 unique numbers, files exist'
    } catch {
        Write-Fail ('parallel new: ' + $_.Exception.Message)
    }

    # ---- Summary ----
    Write-Host ''
    $total = $script:passes + $script:fails
    if ($script:fails -eq 0) {
        Write-Host ("SMOKE RESULT: PASS ({0}/{1})" -f $script:passes, $total)
    } else {
        Write-Host ("SMOKE RESULT: FAIL ({0} failed)" -f $script:fails)
    }
} finally {
    foreach ($c in $cleanup) {
        if ($c -and (Test-Path -LiteralPath $c)) {
            Remove-Item -LiteralPath $c -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    if ($prevTicketsDir) {
        $env:TICKETS_DIR = $prevTicketsDir
    } else {
        Remove-Item Env:TICKETS_DIR -ErrorAction SilentlyContinue
    }
}

if ($script:fails -gt 0) { exit 1 }
exit 0
