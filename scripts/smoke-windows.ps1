# smoke-windows.ps1 - Windows smoke test for the Go ticket binary (ticket.exe).
#
# Purpose:
#   Verify the cross-compiled Windows binary (dist/ticket.exe, see
#   AGENTS_ARCHITECTURE.md sections 5, 7, 8) on a Windows host (internal Windows test host):
#     1. new   -> creates T-0001-open.md and prints its path
#     2. list  -> shows the ticket
#     3. show  -> prints the ticket body
#     4. set   -> renames to T-0001-wip.md, updates status line + journal
#     5. global flag: -C / --tickets-dir= from a foreign CWD
#     6. TICKETS_DIR env override
#     7. parallel new x5 -> 5 unique sequential numbers (OS lock)
#     8. init  -> tickets/ + tickets/archive/; repeat no-op; file conflict (exit 1, stderr)
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
# Decode native-command output as UTF-8 (Go binary writes UTF-8; PS 5.1 otherwise decodes via OEM code page).
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

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

# Ticket files use Russian UI text (bash-reference compatible). Build the
# Cyrillic fragments we assert on from Unicode code points so that this script
# file stays pure ASCII: the body line checked below is "- Status (<RU 'status'>): wip".
$statLabel = -join [char[]](0x0421, 0x0442, 0x0430, 0x0442, 0x0443, 0x0441)  # RU word for "status"
$statWip = '- Status (' + $statLabel + '): wip'
# Step 8 (init) asserts two more Russian fragments (cmd_init.go): success label
# printed by `ticket init` + conflict label on stderr when a file blocks init.
$initOkMsg = (-join [char[]](0x0418, 0x043D, 0x0438, 0x0446, 0x0438, 0x0430, 0x043B, 0x0438, 0x0437, 0x0438, 0x0440, 0x043E, 0x0432, 0x0430, 0x043D, 0x043E)) + ':'  # RU "initialized:"
$conflictMsg = (-join [char[]](0x041A, 0x043E, 0x043D, 0x0444, 0x043B, 0x0438, 0x043A, 0x0442)) + ':'  # RU "conflict:"

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
    # Sandbox: <temp>\ticket-smoke-<guid>\bin\ticket.exe
    try {
        $sb = Join-Path $env:TEMP ('ticket-smoke-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $sb 'tickets') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $sb 'bin') -Force | Out-Null
        $cleanup += $sb
        Copy-Item -LiteralPath $TicketExe -Destination (Join-Path $sb 'bin\ticket.exe')
    } catch {
        Write-Host ("[FAIL] sandbox setup: " + $_.Exception.Message)
        Write-Host "SMOKE RESULT: FAIL (sandbox setup)"
        exit 1
    }
    $exe = Join-Path $sb 'bin\ticket.exe'
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

    # ---- Step 5: foreign CWD + global flag (-C / --tickets-dir) ----
    # NOTE: the real Windows run on the test host is tracked by T-0055,
    # not by this change; this script only encodes the expected scenario.
    try {
        $r = Invoke-Ticket -ExePath $exe -Arguments @('-C', $ticketsDir, 'list') -WorkingDirectory $env:TEMP
        Assert-True ($r.ExitCode -eq 0) ("-C exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match 'T-0001') 'ticket not visible via -C from a foreign CWD'
        $r2 = Invoke-Ticket -ExePath $exe -Arguments @(("--tickets-dir=" + $ticketsDir), 'list') -WorkingDirectory $env:TEMP
        Assert-True ($r2.ExitCode -eq 0) ("--tickets-dir= exit code " + $r2.ExitCode + " (expected 0)")
        Assert-True ($r2.Stdout -match 'T-0001') 'ticket not visible via --tickets-dir= from a foreign CWD'
        Write-Pass 'foreign CWD: -C and --tickets-dir resolve the tickets dir'
    } catch {
        Write-Fail ('foreign CWD flag: ' + $_.Exception.Message)
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
        # Priority: flag must beat the env override. With TICKETS_DIR still
        # pointing at $envT, -C $ticketsDir must list the sandbox ticket
        # (status wip / title 'smoke test ticket'); a bare T-0001 match would
        # not discriminate (the env ticket is T-0001 too, inside $envT).
        $r = Invoke-Ticket -ExePath $exe -Arguments @('-C', $ticketsDir, 'list') -WorkingDirectory $sb
        Assert-True ($r.ExitCode -eq 0) ("flag>env exit code " + $r.ExitCode + " (expected 0)")
        Assert-True (($r.Stdout -match 'wip') -or ($r.Stdout -match 'smoke test ticket')) 'flag > TICKETS_DIR: sandbox ticket not listed via -C while env override is set'
        Write-Pass 'flag > env: -C wins over TICKETS_DIR'
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

    # ---- Step 8: init (create tree / no-op repeat / conflict on file) ----
    try {
        # Sub-case 1: clean dir -> tickets/ + archive/ created, usable for new/list.
        $initDir = Join-Path $env:TEMP ('ticket-smoke-init-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $initDir -Force | Out-Null
        $cleanup += $initDir
        $r = Invoke-Ticket -ExePath $exe -Arguments @('init') -WorkingDirectory $initDir
        Assert-True ($r.ExitCode -eq 0) ("init exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match $initOkMsg) ("init stdout missing RU 'initialized' label: '" + $r.Stdout + "'")
        $initTickets = Join-Path $initDir 'tickets'
        $initArchive = Join-Path $initTickets 'archive'
        Assert-True (Test-Path -LiteralPath $initTickets -PathType Container) ("tickets dir not created: " + $initTickets)
        Assert-True (Test-Path -LiteralPath $initArchive -PathType Container) ("archive dir not created: " + $initArchive)
        $r = Invoke-Ticket -ExePath $exe -Arguments @('new', 'smoke init ticket', '-t', 'OPS', '-p', 'low', '-d', 'init smoke') -WorkingDirectory $initDir
        Assert-True ($r.ExitCode -eq 0) ("init-tree new exit code " + $r.ExitCode + " (expected 0)")
        Assert-True (Test-Path -LiteralPath (Join-Path $initTickets 'T-0001-open.md')) 'T-0001-open.md not created in init tree'
        $r = Invoke-Ticket -ExePath $exe -Arguments @('list') -WorkingDirectory $initDir
        Assert-True ($r.ExitCode -eq 0) ("init-tree list exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match 'T-0001') 'T-0001 not visible in init tree listing'
        Write-Pass 'init: clean dir -> tickets/ + archive/ created, new/list see T-0001'

        # Sub-case 2: repeat init is a no-op - still exit 0, same message.
        $r = Invoke-Ticket -ExePath $exe -Arguments @('init') -WorkingDirectory $initDir
        Assert-True ($r.ExitCode -eq 0) ("repeat init exit code " + $r.ExitCode + " (expected 0)")
        Assert-True ($r.Stdout -match $initOkMsg) ("repeat init stdout missing RU 'initialized' label: '" + $r.Stdout + "'")
        Write-Pass 'init: repeat run is a no-op, exit 0'

        # Sub-case 3: a regular file named 'tickets' blocks init - exit 1,
        # empty stdout, conflict label on stderr, file untouched. Direct .NET
        # Process APIs (as in step 7): Invoke-Ticket captures only stdout.
        $confDir = Join-Path $env:TEMP ('ticket-smoke-initc-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $confDir -Force | Out-Null
        $cleanup += $confDir
        $confFile = Join-Path $confDir 'tickets'
        Set-Content -LiteralPath $confFile -Value 'existing' -Encoding Ascii -NoNewline
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $exe
        $psi.Arguments = 'init'
        $psi.WorkingDirectory = $confDir
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.StandardOutputEncoding = [System.Text.Encoding]::UTF8
        $psi.StandardErrorEncoding = [System.Text.Encoding]::UTF8
        $p = [System.Diagnostics.Process]::Start($psi)
        if (-not $p.WaitForExit(30000)) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
            throw 'init process did not exit within 30 s'
        }
        $confOut = $p.StandardOutput.ReadToEnd().Trim()
        $confErr = $p.StandardError.ReadToEnd().Trim()
        Assert-True ($p.ExitCode -eq 1) ("conflict init exit code " + $p.ExitCode + " (expected 1)")
        Assert-True ($confOut.Length -eq 0) ("conflict init stdout not empty: '" + $confOut + "'")
        Assert-True ($confErr -match $conflictMsg) ("conflict init stderr missing RU 'conflict' label: '" + $confErr + "'")
        Assert-True ((Get-Content -LiteralPath $confFile -Raw) -ceq 'existing') 'conflicting tickets file was modified by init'
        Write-Pass 'init: file named tickets -> exit 1, stderr conflict label, file untouched'
    } catch {
        Write-Fail ('init: ' + $_.Exception.Message)
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
