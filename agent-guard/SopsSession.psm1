# Load the SOPS age identity into one terminal, so sops, git diff and
# generate-talos.ps1 stop asking 1Password once per file. Processes started
# from that terminal inherit the key: do not start an AI agent from it.
#
# Installed by agent-guard/install.ps1. PowerShell loads it on first use.

$lockEvent = 'SopsSession.Lock'

function Get-SopsKeyCommand {
    $command = [Environment]::GetEnvironmentVariable('SOPS_AGE_KEY_CMD', 'User')
    if (-not $command) { $command = $env:SOPS_AGE_KEY_CMD }
    if (-not $command) { throw 'SOPS_AGE_KEY_CMD is not set. Run agent-guard/install.ps1 -SopsKeyCommand "op read op://..."' }
    $command
}

function Unlock-Sops {
    <#
    .SYNOPSIS
    Load the SOPS age identity into this terminal for a limited time.
    #>
    [CmdletBinding()]
    param(
        # Clear the key again after this many minutes.
        [ValidateRange(1, 720)]
        [int] $Minutes = 15
    )
    if ($env:CLAUDECODE -or $env:CODEX_SANDBOX -or $env:CODEX_THREAD_ID) {
        throw 'Unlock-Sops runs only in your own terminal, not in an agent session.'
    }
    # Same parsing as sops: whitespace-separated words, no shell.
    $words = @((Get-SopsKeyCommand) -split '\s+' | Where-Object { $_ })
    $identity = & $words[0] @($words | Select-Object -Skip 1)
    if ($LASTEXITCODE -ne 0 -or -not ($identity -match '^AGE-(SECRET-KEY|PLUGIN)-')) {
        throw "$($words[0]) did not return an age identity."
    }
    $env:SOPS_AGE_KEY = $identity -join "`n"
    # Otherwise sops would also run the command, and prompt, on every call.
    $env:SOPS_AGE_KEY_CMD = $null

    Get-EventSubscriber -SourceIdentifier $lockEvent -ErrorAction SilentlyContinue | Unregister-Event
    $timer = [Timers.Timer]::new($Minutes * 60000)
    $timer.AutoReset = $false
    Register-ObjectEvent -InputObject $timer -EventName Elapsed -SourceIdentifier $lockEvent -Action {
        $env:SOPS_AGE_KEY = $null
        $env:SOPS_AGE_KEY_CMD = [Environment]::GetEnvironmentVariable('SOPS_AGE_KEY_CMD', 'User')
        Write-Host "`nSOPS key cleared from this terminal."
    } | Out-Null
    $timer.Start()
    Write-Host "SOPS key loaded in this terminal for $Minutes minutes. Lock-Sops clears it sooner."
}

function Lock-Sops {
    <#
    .SYNOPSIS
    Clear the SOPS age identity from this terminal.
    #>
    [CmdletBinding()]
    param()
    $env:SOPS_AGE_KEY = $null
    $env:SOPS_AGE_KEY_CMD = [Environment]::GetEnvironmentVariable('SOPS_AGE_KEY_CMD', 'User')
    Get-EventSubscriber -SourceIdentifier $lockEvent -ErrorAction SilentlyContinue | Unregister-Event
    Write-Host 'SOPS key cleared from this terminal.'
}

Export-ModuleMember -Function Unlock-Sops, Lock-Sops
