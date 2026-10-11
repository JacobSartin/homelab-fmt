#Requires -Version 7.2
# Runs install.ps1 against a scratch root and checks what it writes.
$ErrorActionPreference = 'Stop'
$installer = Join-Path $PSScriptRoot 'install.ps1'
$failures = [Collections.Generic.List[string]]::new()
function Assert([bool] $Condition, [string] $Message) {
    if ($Condition) { Write-Host "ok   $Message" } else { Write-Host "FAIL $Message"; $failures.Add($Message) }
}

$root = Join-Path ([IO.Path]::GetTempPath()) ('agent-guard-test-' + [guid]::NewGuid().ToString('N'))
try {
    $homeDir = Join-Path $root 'home'
    $homelab = Join-Path $root 'homelab'
    foreach ($dir in "$homeDir\.claude", "$homeDir\.codex", "$homeDir\AppData\Roaming\Code\User", "$homelab\cluster", "$homelab\infrastructure") {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    Set-Content -LiteralPath "$homeDir\.claude\settings.json" -Value (@{
            model = 'opus'
            env = [ordered]@{ GIT_CONFIG_COUNT = '1'; GIT_CONFIG_KEY_0 = 'diff.sopsdiffer.textconv'; GIT_CONFIG_VALUE_0 = 'cat' }
            hooks = @{ Stop = @(@{ hooks = @(@{ type = 'command'; command = 'echo done' }) }) }
        } | ConvertTo-Json -Depth 8)
    Set-Content -LiteralPath "$homeDir\.codex\config.toml" -Value "model = `"x`"`ndefault_permissions = `"workspace-winget`"`n`n[permissions.workspace-winget]`nextends = `":workspace`"`n"
    Set-Content -LiteralPath "$homeDir\AppData\Roaming\Code\User\settings.json" -Value "{`n  `"editor.tabSize`": 2,`n  // comment`n  `"files.exclude`": { `"**/.x`": true, },`n}`n"

    $common = @{ TestRoot = $root; ProtectedRoot = $homelab; ExemptRoot = "$homelab\infrastructure"; SopsKeyCommand = 'op read op://Homelab/sops-age/notesPlain' }
    & $installer @common -Machine | Out-Null

    $settings = Get-Content -Raw "$homeDir\.claude\settings.json" | ConvertFrom-Json -AsHashtable
    Assert ($settings.model -eq 'opus') 'Claude settings keep unrelated keys'
    Assert ($settings.hooks.Stop.Count -eq 1) 'Claude settings keep unrelated hooks'
    Assert ($settings.permissions.deny -contains 'Read(//**/*.agekey)') 'Claude settings deny reading key files'
    Assert ($settings.permissions.deny -contains 'Read(~/AppData/Roaming/Code/Backups/**)') 'Claude settings deny reading VS Code backups'
    Assert ($settings.hooks.PreToolUse.Count -eq 1 -and $settings.hooks.PreToolUse[0].hooks[0].command -match 'agent-guard/secret-guard\.mjs"$') 'PreToolUse hook installed once'
    Assert ($settings.hooks.SessionStart[0].hooks[0].command -match '--session-start$') 'SessionStart hook installed'
    Assert ($settings.env.GIT_CONFIG_COUNT -eq '1') 'existing git diff override reused, not duplicated'
    Assert ((Get-Content -Raw "$homeDir\.claude\settings.json.agent-guard.bak") -match '"model": "opus"' -and
        (Get-Content -Raw "$homeDir\.claude\settings.json.agent-guard.bak") -notmatch 'agent-guard') 'the original settings are backed up'

    $hook = Get-Content -Raw "$homeDir\.claude\agent-guard\secret-guard.mjs"
    Assert ($hook -match 'agent-guard version: [0-9a-f]{12}') 'hook copy carries the version'
    Assert ($hook -match [regex]::Escape("const EXEMPT_ROOTS = [`"$(($homelab -replace '\\', '/'))/infrastructure`"];")) 'hook copy carries the exempt roots'

    $managed = Get-Content -Raw "$homeDir\.codex\config.toml"
    Assert ($managed -match '(?m)^default_permissions = "agent-guard"  # agent-guard \(was "workspace-winget"\)$') 'Codex selects the guard profile and remembers the old one'
    Assert ($managed -match 'extends = "workspace-winget"') 'Codex profile extends the user profile'
    Assert ($managed -match '(?m)^model = "x"$' -and $managed -match '(?m)^\[permissions\.workspace-winget\]$') 'Codex config keeps the user settings'
    Assert ($managed -match '/cluster/\*\*/\*\.agekey" = "deny"') 'Codex denies key files in protected repositories'
    Assert ($managed -notmatch '/infrastructure/\*\*/\*\.agekey') 'Codex leaves key files in exempt repositories'
    Assert ($managed -match '/infrastructure/\*\*/\.decrypted~\*" = "deny"') 'Codex denies decrypted copies in exempt repositories too'
    Assert ($managed -match 'exclude = \["SOPS_AGE_\*"\]') 'Codex filters SOPS key variables'
    Assert (([regex]::Matches($managed, 'diff\.sopsdiffer\.textconv')).Count -eq 1) 'Codex git diff override added once'

    # Codex config variants: an existing [shell_environment_policy.set], a bare file, an existing policy table.
    $variants = [ordered]@{
        'set table' = "default_permissions = `"mine`"`n`n[permissions.mine]`nextends = `":workspace`"`n`n[shell_environment_policy.set]`nGIT_CONFIG_COUNT = `"1`"`nGIT_CONFIG_KEY_0 = `"diff.sopsdiffer.textconv`"`nGIT_CONFIG_VALUE_0 = `"cat`"`n"
        'empty' = ''
        'policy table' = "[shell_environment_policy]`ninherit = `"all`"`n"
    }
    foreach ($name in $variants.Keys) {
        Set-Content -LiteralPath "$homeDir\.codex\config.toml" -Value $variants[$name] -NoNewline
        & $installer @common | Out-Null
        $first = Get-Content -Raw "$homeDir\.codex\config.toml"
        & $installer @common | Out-Null
        $second = Get-Content -Raw "$homeDir\.codex\config.toml"
        Assert ($first -eq $second) "Codex config ($name): a rerun changes nothing"
        Assert (([regex]::Matches($first, '(?m)^default_permissions')).Count -eq 1) "Codex config ($name): one default profile"
        Assert (([regex]::Matches($first, 'SOPS_AGE_\*')).Count -eq 1) "Codex config ($name): one SOPS filter"
        Assert (([regex]::Matches($first, 'diff\.sopsdiffer\.textconv')).Count -eq 1) "Codex config ($name): one git diff override"
        if (Get-Command codex -ErrorAction SilentlyContinue) {
            # Codex must still load it. Helpers refuse a CODEX_HOME under %TEMP%, so copy it out.
            $probe = Join-Path $env:LOCALAPPDATA ('agent-guard-codex-' + [guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory $probe | Out-Null
            Copy-Item "$homeDir\.codex\config.toml" "$probe\config.toml"
            $saved = $env:CODEX_HOME
            $env:CODEX_HOME = $probe
            try { & codex features list *> $null; $loaded = $LASTEXITCODE -eq 0 } finally { $env:CODEX_HOME = $saved; Remove-Item -LiteralPath $probe -Recurse -Force }
            Assert $loaded "Codex config ($name): codex loads it"
        }
    }
    Set-Content -LiteralPath "$homeDir\.codex\config.toml" -Value "model = `"x`"`ndefault_permissions = `"workspace-winget`"`n`n[permissions.workspace-winget]`nextends = `":workspace`"`n"
    & $installer @common -Machine | Out-Null

    $vscode = Get-Content -Raw "$homeDir\AppData\Roaming\Code\User\settings.json"
    Assert ($vscode -match '"workbench\.localHistory\.exclude": \{ "\*\*/\.decrypted~\*": true \}') 'VS Code Local History skips decrypted copies'
    Assert ($vscode -match '// comment' -and $vscode -match '"\*\*/\.x": true, \}') 'VS Code settings keep comments and layout'

    Assert (Test-Path "$homeDir\Documents\PowerShell\Modules\SopsSession\SopsSession.psm1") 'SopsSession module installed'
    Assert ((Get-Content -Raw "$root\user-environment.txt") -match 'SOPS_AGE_KEY_CMD=op read') 'SOPS_AGE_KEY_CMD recorded'

    $managedSettings = Get-Content -Raw "$root\Program Files\ClaudeCode\managed-settings.json" | ConvertFrom-Json -AsHashtable
    Assert ($managedSettings.hooks.PreToolUse[0].hooks[0].command -match 'ClaudeCode/agent-guard/secret-guard\.mjs') 'machine Claude hook points at the machine copy'
    $requirements = Get-Content -Raw "$root\ProgramData\OpenAI\Codex\requirements.toml"
    Assert ($requirements -match 'deny_read = \[' -and $requirements -match 'prefix_rules = \[') 'Codex requirements deny reads and add rules'

    & $installer @common -Machine -Check | Out-Null
    Assert ($LASTEXITCODE -eq 0) 'a second run with -Check finds nothing to change'

    Set-Content -LiteralPath "$homeDir\.codex\rules\agent-guard.rules" -Value 'tampered'
    & $installer @common -Machine -Check | Out-Null
    Assert ($LASTEXITCODE -eq 1) '-Check reports a changed file'

    $codex = Get-Command codex -ErrorAction SilentlyContinue
    if ($codex) {
        & $installer @common | Out-Null
        $decision = { param($cmd) ((& codex execpolicy check --rules "$homeDir\.codex\rules\agent-guard.rules" -- @cmd) | ConvertFrom-Json).decision }
        Assert ((& $decision @('sops', '-d', 'x.sops.yaml')) -eq 'prompt') 'Codex rules prompt for sops decryption'
        Assert ((& $decision @('op', 'read', 'op://v/i/f')) -eq 'forbidden') 'Codex rules forbid the 1Password CLI'
        Assert ((& $decision @('kubectl.exe', 'get', 'secrets')) -eq 'forbidden') 'Codex rules forbid reading Secrets'
        Assert ($null -eq (& $decision @('kubectl', 'get', 'pods'))) 'Codex rules leave other commands alone'
    } else {
        Write-Host 'skip Codex rule checks: codex is not installed'
    }
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
if ($failures.Count) { throw "$($failures.Count) installer test(s) failed." }
