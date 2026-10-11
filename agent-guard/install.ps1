#Requires -Version 7.2
<#
.SYNOPSIS
Installs agent-guard, which keeps Claude Code and Codex away from SOPS keys and
decrypted secrets on this workstation.

.DESCRIPTION
Every run writes the user copies:
  ~/.claude/agent-guard/        hook script and its settings
  ~/.claude/settings.json       deny rules and hooks (merged; other keys kept)
  ~/.codex/config.toml          deny-read permission profile and environment filter
                                (a marked block; the rest of the file is kept)
  ~/.codex/rules/agent-guard.rules
  VS Code user settings         Local History skips .decrypted~ files
  PowerShell module SopsSession Unlock-Sops and Lock-Sops
-Machine (elevated) also writes copies that only administrators can change:
  %ProgramFiles%\ClaudeCode\managed-settings.json and agent-guard\
  %ProgramData%\OpenAI\Codex\requirements.toml
-Check reports what differs without writing. Rerun after pulling changes.
#>
[CmdletBinding()]
param(
    [switch] $Machine,
    [switch] $Check,
    # Directories whose repositories must not expose keys or decrypted files.
    [string[]] $ProtectedRoot = @('D:\git\homelab'),
    # Repositories where agents may still read local key files. Temporary.
    [string[]] $ExemptRoot = @('D:\git\homelab\infrastructure'),
    # Command sops runs to obtain the age identity, stored as the user-level
    # SOPS_AGE_KEY_CMD, e.g. 'op read op://Homelab/sops-age/notesPlain'.
    [string] $SopsKeyCommand,
    # Redirect every target below this directory. Used by the tests.
    [string] $TestRoot
)

$ErrorActionPreference = 'Stop'
$source = $PSScriptRoot

# --- Targets ---------------------------------------------------------------

if ($TestRoot) {
    $homeDir = Join-Path $TestRoot 'home'
    $appData = Join-Path $homeDir 'AppData\Roaming'
    $documents = Join-Path $homeDir 'Documents'
    $programFiles = Join-Path $TestRoot 'Program Files'
    $programData = Join-Path $TestRoot 'ProgramData'
    $codexHome = Join-Path $homeDir '.codex'
} else {
    $homeDir = [Environment]::GetFolderPath('UserProfile')
    $appData = [Environment]::GetFolderPath('ApplicationData')
    $documents = [Environment]::GetFolderPath('MyDocuments')
    $programFiles = $env:ProgramFiles
    $programData = $env:ProgramData
    $codexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $homeDir '.codex' }
}
$claudeHome = Join-Path $homeDir '.claude'
$userGuard = Join-Path $claudeHome 'agent-guard'
$machineClaude = Join-Path $programFiles 'ClaudeCode'
$machineGuard = Join-Path $machineClaude 'agent-guard'
$machineCodex = Join-Path $programData 'OpenAI\Codex'

$normalize = { param($p) [IO.Path]::GetFullPath($p).TrimEnd('\', '/') }
$protected = @($ProtectedRoot | Where-Object { $_ } | ForEach-Object { & $normalize $_ })
$exempt = @($ExemptRoot | Where-Object { $_ } | ForEach-Object { & $normalize $_ })
$slash = { param($p) $p -replace '\\', '/' }

# --- Version ---------------------------------------------------------------

# Content hash of the sources and the options that shape the output.
$hashInput = (@('secret-guard.mjs', 'install.ps1', 'SopsSession.psm1') | ForEach-Object {
        [IO.File]::ReadAllText((Join-Path $source $_))
    }) + ($protected -join ';') + ($exempt -join ';')
$version = [Convert]::ToHexString(
    [Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($hashInput -join "`n"))
).Substring(0, 12).ToLowerInvariant()

# --- What agents must not read ---------------------------------------------

# Repositories under the protected roots, minus the exempt ones. Key files are
# denied per repository so an exempt repository keeps its own keys.
$repositories = @(foreach ($root in $protected) {
        if (Test-Path -LiteralPath $root -PathType Container) {
            # Dot folders hold caches, not repositories; Codex expands every glob per command.
            Get-ChildItem -LiteralPath $root -Directory | Where-Object Name -NotLike '.*' | ForEach-Object { & $normalize $_.FullName }
        }
    })
$keyRepositories = @($repositories | Where-Object { $_ -notin $exempt })

$homeDenials = @(
    '~/AppData/Roaming/sops'
    '~/.config/sops'
    '~/AppData/Roaming/Code/User/History'
    '~/AppData/Roaming/Code/Backups'
)
$keyDenials = @(
    foreach ($root in $protected) { "$(& $slash $root)/*.agekey" }
    foreach ($repo in $keyRepositories) { "$(& $slash $repo)/**/*.agekey" }
)
$copyDenials = @(foreach ($repo in $repositories) {
        "$(& $slash $repo)/**/.decrypted~*"
        "$(& $slash $repo)/**/*sopssecret*"
        "$(& $slash $repo)/**/talos/generated"
    })

$claudeDeny = @(
    foreach ($pattern in '//**/*.agekey', '//**/.decrypted~*', '//**/*sopssecret*', '//**/talos/generated/**') {
        "Read($pattern)"
        "Edit($pattern)"
    }
    'Read(~/AppData/Roaming/sops/**)'
    'Read(~/.config/sops/**)'
    'Read(~/AppData/Roaming/Code/User/History/**)'
    'Read(~/AppData/Roaming/Code/Backups/**)'
    'Edit(~/.claude/settings.json)'
    'Edit(~/.claude/settings.local.json)'
    'Edit(~/.claude/agent-guard/**)'
    'Edit(~/.codex/config.toml)'
    'Edit(~/.codex/managed_config.toml)'
    'Edit(~/.codex/rules/**)'
)

# Codex prefix rules. `prompt` asks the owner; `forbidden` refuses outright.
# sops only prompts so exempt repositories keep working.
$codexRules = @(
    @{ Pattern = @(, @('sops', 'sops.exe')) + @(, @('decrypt', '-d', '--decrypt', 'edit', 'exec-env', 'exec-file', 'set', 'unset', 'rotate', '-r', 'updatekeys', 'keyservice'))
        Decision = 'prompt'; Why = 'sops decryption needs the owner' }
    @{ Pattern = @(, @('op', 'op.exe')) + @(, @('read', 'inject', 'run', 'item', 'document', 'signin', 'account', 'whoami', 'vault', 'service-account', 'connect', 'plugin', 'user'))
        Decision = 'forbidden'; Why = 'the 1Password CLI is reserved for the owner' }
    @{ Pattern = @(, @('Unlock-Sops', 'unlock-sops'))
        Decision = 'forbidden'; Why = 'Unlock-Sops is reserved for the owner' }
    @{ Pattern = @(, @('age', 'age.exe')) + @(, @('-d', '--decrypt', '-i', '--identity'))
        Decision = 'forbidden'; Why = 'age decryption is reserved for the owner' }
    @{ Pattern = @(, @('age-keygen', 'age-keygen.exe', 'age-plugin-yubikey', 'age-plugin-yubikey.exe', 'age-plugin-batchpass', 'age-plugin-batchpass.exe'))
        Decision = 'forbidden'; Why = 'keys are generated and used outside agent sessions' }
    @{ Pattern = @(, @('kubectl', 'kubectl.exe')) + @(, @('get', 'describe', 'edit', 'create')) + @(, @('secret', 'secrets', 'secret/'))
        Decision = 'forbidden'; Why = 'reading Kubernetes Secrets is reserved for the owner' }
    @{ Pattern = @(, @('kubectl', 'kubectl.exe')) + 'config', 'view', '--raw'
        Decision = 'forbidden'; Why = 'kubeconfig credentials are reserved for the owner' }
    @{ Pattern = @(, @('talosctl', 'talosctl.exe')) + @(, @('get', 'g')) + @(, @('mc', 'machineconfig', 'machineconfigs'))
        Decision = 'forbidden'; Why = 'Talos machine configs hold cluster secrets' }
    @{ Pattern = @(, @('talosctl', 'talosctl.exe')) + @(, @('read', 'cat', 'rotate-ca'))
        Decision = 'forbidden'; Why = 'Talos file reads and CA rotation are reserved for the owner' }
    @{ Pattern = @(, @('talosctl', 'talosctl.exe')) + 'gen' + @(, @('secrets', 'config'))
        Decision = 'forbidden'; Why = 'Talos secrets are generated outside agent sessions' }
)

# --- Rendering -------------------------------------------------------------

function ConvertTo-TomlString([string] $Value) {
    '"' + ($Value -replace '\\', '\\' -replace '"', '\"') + '"'
}

function ConvertTo-TomlArray([string[]] $Values) {
    '[' + (($Values | ForEach-Object { ConvertTo-TomlString $_ }) -join ', ') + ']'
}

function Format-StarlarkRules {
    $lines = @("# agent-guard version: $version", '# Generated by agent-guard/install.ps1. Edit the installer, not this file.', '')
    foreach ($rule in $codexRules) {
        $tokens = foreach ($token in $rule.Pattern) {
            if ($token -is [array]) { ConvertTo-TomlArray $token } else { ConvertTo-TomlString $token }
        }
        $lines += 'prefix_rule('
        $lines += "    pattern = [$($tokens -join ', ')],"
        $lines += "    decision = `"$($rule.Decision)`","
        $lines += "    justification = $(ConvertTo-TomlString $rule.Why),"
        $lines += ')'
        $lines += ''
    }
    $lines -join "`n"
}

function Format-TomlRules {
    $entries = foreach ($rule in $codexRules) {
        $tokens = foreach ($token in $rule.Pattern) {
            if ($token -is [array]) { "{ any_of = $(ConvertTo-TomlArray $token) }" } else { "{ token = $(ConvertTo-TomlString $token) }" }
        }
        "  { pattern = [$($tokens -join ', ')], decision = `"$($rule.Decision)`", justification = $(ConvertTo-TomlString $rule.Why) },"
    }
    "[rules]`nprefix_rules = [`n$($entries -join "`n")`n]"
}

# Codex reads permission profiles only from its regular config layers (a
# profile in managed_config.toml is ignored), so the guard edits config.toml: a
# marked block at the end, plus tagged lines where TOML needs them inside the
# user's own tables. A rerun removes both and adds them again.
$blockStart = '# >>> agent-guard'
$blockEnd = '# <<< agent-guard'
$lineTag = '# agent-guard'

function Find-Line($Lines, [string] $Pattern, [int] $From = 0) {
    for ($i = $From; $i -lt $Lines.Count; $i++) { if ($Lines[$i] -match $Pattern) { return $i } }
    -1
}

function Merge-CodexConfig([string] $Text) {
    # Undo the previous install, restoring the user's default profile.
    $lines = [Collections.Generic.List[string]]::new()
    $inBlock = $false
    foreach ($line in @(if ($Text) { ($Text -replace "`r`n", "`n").TrimEnd("`n") -split "`n" })) {
        if ($line.StartsWith($blockStart)) { $inBlock = $true; continue }
        if ($inBlock) { if ($line.StartsWith($blockEnd)) { $inBlock = $false }; continue }
        if ($line -match "^default_permissions = `"agent-guard`"\s+$([regex]::Escape($lineTag)) \(was `"([^`"]+)`"\)$") {
            $lines.Add("default_permissions = `"$($Matches[1])`"")
            continue
        }
        if ($line.EndsWith($lineTag)) { continue }
        $lines.Add($line)
    }
    while ($lines.Count -and $lines[$lines.Count - 1] -eq '') { $lines.RemoveAt($lines.Count - 1) }

    # Select the guard profile, extending whatever the user selected.
    $firstTable = Find-Line $lines '^\s*\['
    if ($firstTable -lt 0) { $firstTable = $lines.Count }
    $default = Find-Line $lines '^\s*default_permissions\s*=\s*"([^"]+)"'
    $base = ':workspace'
    if ($default -ge 0 -and $default -lt $firstTable) {
        $null = $lines[$default] -match '"([^"]+)"'
        $base = $Matches[1]
        $lines[$default] = "default_permissions = `"agent-guard`"  $lineTag (was `"$base`")"
    } else {
        $lines.Insert($firstTable, "default_permissions = `"agent-guard`"  $lineTag (was `"$base`")")
    }

    $extra = [Collections.Generic.List[string]]::new()
    # Keep SOPS key variables out of sandboxed commands.
    $filters = Find-Line $lines '^\s*\[shell_environment_policy\.filters\]'
    $policy = Find-Line $lines '^\s*\[shell_environment_policy\]'
    $set = Find-Line $lines '^\s*\[shell_environment_policy\.set\]'
    if ($filters -ge 0) {
        $lines.Insert($filters + 1, "`"SOPS_AGE_*`" = `"exclude`"  $lineTag")
    } elseif ($policy -ge 0) {
        $next = Find-Line $lines '^\s*\[' ($policy + 1)
        $existing = Find-Line $lines '^\s*exclude\s*=' ($policy + 1)
        if ($existing -ge 0 -and ($next -lt 0 -or $existing -lt $next)) {
            Write-Warning 'config.toml already sets shell_environment_policy.exclude; add "SOPS_AGE_*" to it by hand.'
        } else {
            $lines.Insert($policy + 1, "exclude = [`"SOPS_AGE_*`"]  $lineTag")
        }
    } elseif ($set -ge 0) {
        $lines.Insert($set, "exclude = [`"SOPS_AGE_*`"]  $lineTag")
        $lines.Insert($set, "[shell_environment_policy]  $lineTag")
    } else {
        $extra.AddRange([string[]]@('', '[shell_environment_policy]', 'exclude = ["SOPS_AGE_*"]'))
    }

    # Show SOPS ciphertext, not decrypted values, in git diff.
    if (-not ($lines -match 'diff\.sopsdiffer\.textconv')) {
        $set = Find-Line $lines '^\s*\[shell_environment_policy\.set\]'
        $git = @('GIT_CONFIG_COUNT = "1"', 'GIT_CONFIG_KEY_0 = "diff.sopsdiffer.textconv"', 'GIT_CONFIG_VALUE_0 = "cat"')
        if ($set -lt 0) {
            $extra.AddRange([string[]](@('', '[shell_environment_policy.set]') + $git))
        } elseif ($lines -match '^\s*GIT_CONFIG_COUNT\s*=') {
            Write-Warning 'config.toml already sets GIT_CONFIG_COUNT; add the diff.sopsdiffer.textconv=cat override by hand.'
        } else {
            for ($i = $git.Count - 1; $i -ge 0; $i--) { $lines.Insert($set + 1, "$($git[$i])  $lineTag") }
        }
    }

    $absolute = @($homeDenials + $keyDenials + $copyDenials | ForEach-Object { "$(ConvertTo-TomlString $_) = `"deny`"" })
    $block = @"
$blockStart $version`: managed by homelab-tools/agent-guard/install.ps1; edit the installer, not this block.
[permissions.agent-guard]
extends = $(ConvertTo-TomlString $base)
description = "$base without SOPS keys, decrypted SOPS copies, generated Talos configs or VS Code history"

[permissions.agent-guard.filesystem]
glob_scan_max_depth = 12
$($absolute -join "`n")

[permissions.agent-guard.filesystem.":workspace_roots"]
"**/.decrypted~*" = "deny"
"**/*sopssecret*" = "deny"
"**/talos/generated" = "deny"
$($extra -join "`n")
$blockEnd
"@
    $body = $lines -join "`n"
    if ($body) { "$body`n`n$block`n" } else { "$block`n" }
}

function Format-CodexRequirements {
    $denials = @($homeDenials + $keyDenials + $copyDenials | ForEach-Object { "  $(ConvertTo-TomlString $_)," })
    @"
# Managed by agent-guard (homelab-tools/agent-guard/install.ps1 -Machine).
# agent-guard version: $version

[permissions.filesystem]
deny_read = [
$($denials -join "`n")
]

$(Format-TomlRules)
"@
}

function Get-HookCommand([string] $GuardDirectory, [string] $Argument) {
    $script = & $slash (Join-Path $GuardDirectory 'secret-guard.mjs')
    if ($Argument) { "node `"$script`" $Argument" } else { "node `"$script`"" }
}

function Get-GuardHooks([string] $GuardDirectory) {
    [ordered]@{
        PreToolUse = @([ordered]@{
                matcher = 'Bash|PowerShell|Read|Grep|Glob|Edit|Write|MultiEdit|NotebookEdit'
                hooks = @([ordered]@{ type = 'command'; command = (Get-HookCommand $GuardDirectory); timeout = 30 })
            })
        SessionStart = @([ordered]@{
                hooks = @([ordered]@{ type = 'command'; command = (Get-HookCommand $GuardDirectory '--session-start'); timeout = 30 })
            })
    }
}

# Merge the guard into Claude settings, keeping every other key and hook.
function Merge-ClaudeSettings([string] $Json, [string] $GuardDirectory) {
    $settings = if ($Json) { $Json | ConvertFrom-Json -AsHashtable } else { [ordered]@{} }
    if (-not $settings) { $settings = [ordered]@{} }

    if (-not $settings.Contains('permissions')) { $settings['permissions'] = [ordered]@{} }
    $deny = [Collections.Generic.List[string]]::new()
    foreach ($rule in @($settings['permissions']['deny'])) { if ($rule) { $deny.Add($rule) } }
    foreach ($rule in $claudeDeny) { if (-not $deny.Contains($rule)) { $deny.Add($rule) } }
    $settings['permissions']['deny'] = $deny.ToArray()

    if (-not $settings.Contains('hooks')) { $settings['hooks'] = [ordered]@{} }
    $ours = Get-GuardHooks $GuardDirectory
    foreach ($event in $ours.Keys) {
        $kept = @(@($settings['hooks'][$event]) | Where-Object {
                $_ -and -not (@($_['hooks']) | Where-Object { $_['command'] -match 'agent-guard[\\/]+secret-guard\.mjs' })
            })
        $settings['hooks'][$event] = @($kept) + @($ours[$event])
    }

    # Agents see SOPS ciphertext in git diff instead of running the decrypting driver.
    if (-not $settings.Contains('env')) { $settings['env'] = [ordered]@{} }
    $variables = $settings['env']
    $count = [int]($variables['GIT_CONFIG_COUNT'] ?? 0)
    $present = $false
    for ($i = 0; $i -lt $count; $i++) {
        if ($variables["GIT_CONFIG_KEY_$i"] -eq 'diff.sopsdiffer.textconv') { $variables["GIT_CONFIG_VALUE_$i"] = 'cat'; $present = $true }
    }
    if (-not $present) {
        $variables["GIT_CONFIG_KEY_$count"] = 'diff.sopsdiffer.textconv'
        $variables["GIT_CONFIG_VALUE_$count"] = 'cat'
        $variables['GIT_CONFIG_COUNT'] = [string]($count + 1)
    }

    ($settings | ConvertTo-Json -Depth 32) + "`n"
}

# Add a Local History exclusion without rewriting the rest of the JSONC file.
function Merge-VsCodeSettings([string] $Text) {
    if (-not $Text) { return "{`n  `"workbench.localHistory.exclude`": { `"**/.decrypted~*`": true },`n}`n" }
    if ($Text -match '\.decrypted~') { return $Text }
    if ($Text -match '"workbench\.localHistory\.exclude"\s*:\s*\{') {
        return [regex]::new('("workbench\.localHistory\.exclude"\s*:\s*\{)').Replace($Text, "`$1 `"**/.decrypted~*`": true,", 1)
    }
    [regex]::new('^\s*\{').Replace($Text, "{`n  `"workbench.localHistory.exclude`": { `"**/.decrypted~*`": true },", 1)
}

# --- Writing ---------------------------------------------------------------

$results = [Collections.Generic.List[object]]::new()

function Set-ManagedFile([string] $Path, [string] $Content) {
    $current = if (Test-Path -LiteralPath $Path) { [IO.File]::ReadAllText($Path) } else { $null }
    $state = if ($current -eq $Content) { 'current' } elseif ($null -eq $current) { 'missing' } else { 'differs' }
    if (-not $Check -and $state -ne 'current') {
        New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
        # Keep the file as it was before agent-guard first touched it.
        $backup = "$Path.agent-guard.bak"
        if ($null -ne $current -and -not (Test-Path -LiteralPath $backup)) { [IO.File]::WriteAllText($backup, $current) }
        [IO.File]::WriteAllText($Path, $Content, [Text.UTF8Encoding]::new($false))
        $state = if ($state -eq 'missing') { 'created' } else { 'updated' }
    }
    $results.Add([pscustomobject]@{ Target = $Path; State = $state })
}

function Read-Text([string] $Path) {
    if (Test-Path -LiteralPath $Path) { [IO.File]::ReadAllText($Path) } else { $null }
}

function Get-HookSource {
    [IO.File]::ReadAllText((Join-Path $source 'secret-guard.mjs')).Replace(
        '// agent-guard version: dev', "// agent-guard version: $version").Replace(
        'const EXEMPT_ROOTS = [];', "const EXEMPT_ROOTS = $(ConvertTo-Json -Compress -InputObject @($exempt | ForEach-Object { & $slash $_ }));")
}

if ($Machine -and -not $TestRoot -and -not $Check) {
    $principal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'install.ps1 -Machine must run from an elevated PowerShell.'
    }
}

# User scope.
Set-ManagedFile (Join-Path $userGuard 'secret-guard.mjs') (Get-HookSource)
$claudeSettings = Join-Path $claudeHome 'settings.json'
Set-ManagedFile $claudeSettings (Merge-ClaudeSettings (Read-Text $claudeSettings) $userGuard)
$codexConfig = Join-Path $codexHome 'config.toml'
Set-ManagedFile $codexConfig (Merge-CodexConfig (Read-Text $codexConfig))
# Earlier versions wrote managed_config.toml, whose profile Codex ignores.
$oldManaged = Join-Path $codexHome 'managed_config.toml'
if (-not $Check -and (Read-Text $oldManaged) -match 'agent-guard version') {
    Remove-Item -LiteralPath $oldManaged
    $results.Add([pscustomobject]@{ Target = $oldManaged; State = 'removed' })
}
Set-ManagedFile (Join-Path $codexHome 'rules\agent-guard.rules') (Format-StarlarkRules)
$vscode = Join-Path $appData 'Code\User\settings.json'
Set-ManagedFile $vscode (Merge-VsCodeSettings (Read-Text $vscode))
Set-ManagedFile (Join-Path $documents 'PowerShell\Modules\SopsSession\SopsSession.psm1') (
    [IO.File]::ReadAllText((Join-Path $source 'SopsSession.psm1')))

if ($SopsKeyCommand) {
    if ($TestRoot) {
        Set-ManagedFile (Join-Path $TestRoot 'user-environment.txt') "SOPS_AGE_KEY_CMD=$SopsKeyCommand`n"
    } else {
        $current = [Environment]::GetEnvironmentVariable('SOPS_AGE_KEY_CMD', 'User')
        $state = if ($current -eq $SopsKeyCommand) { 'current' } elseif ($Check) { 'differs' } else { 'updated' }
        if ($state -eq 'updated') { [Environment]::SetEnvironmentVariable('SOPS_AGE_KEY_CMD', $SopsKeyCommand, 'User') }
        $results.Add([pscustomobject]@{ Target = 'User environment SOPS_AGE_KEY_CMD'; State = $state })
    }
}

# Machine scope.
if ($Machine) {
    Set-ManagedFile (Join-Path $machineGuard 'secret-guard.mjs') (Get-HookSource)
    $managed = Join-Path $machineClaude 'managed-settings.json'
    Set-ManagedFile $managed (Merge-ClaudeSettings (Read-Text $managed) $machineGuard)
    if (-not $Check -and -not $TestRoot) {
        # ProgramData lets users create folders; only administrators may change this one.
        New-Item -ItemType Directory -Path $machineCodex -Force | Out-Null
        $acl = [Security.AccessControl.DirectorySecurity]::new()
        $acl.SetAccessRuleProtection($true, $false)
        $inherit = [Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
        foreach ($rule in @(
                @('S-1-5-18', 'FullControl'), @('S-1-5-32-544', 'FullControl'), @('S-1-5-32-545', 'ReadAndExecute'))) {
            $sid = [Security.Principal.SecurityIdentifier]::new($rule[0])
            $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, $rule[1], $inherit, 'None', 'Allow'))
        }
        Set-Acl -LiteralPath $machineCodex -AclObject $acl
    }
    Set-ManagedFile (Join-Path $machineCodex 'requirements.toml') (Format-CodexRequirements)
}

# --- Report ----------------------------------------------------------------

$results | Format-Table -AutoSize | Out-String -Width 200 | Write-Host
foreach ($name in 'SOPS_AGE_KEY', 'SOPS_AGE_KEY_FILE') {
    foreach ($scope in 'User', 'Machine') {
        if (-not $TestRoot -and [Environment]::GetEnvironmentVariable($name, $scope)) {
            Write-Warning "$name is set in the $scope environment; remove it so agents cannot inherit a key."
        }
    }
}
if (-not $Machine -and -not $TestRoot -and -not (Test-Path -LiteralPath (Join-Path $machineGuard 'secret-guard.mjs'))) {
    Write-Warning 'The machine copy is not installed; run install.ps1 -Machine from an elevated PowerShell.'
}
exit ($Check -and ($results | Where-Object State -ne 'current')) ? 1 : 0
