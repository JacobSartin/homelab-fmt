# agent-guard

Keeps Claude Code and Codex from reading SOPS age keys and decrypted secrets
on a Windows workstation. sops, VS Code decrypt-on-open and decrypted
`git diff` keep working for the owner.

## How secrets are held

| Layer | What it does |
| --- | --- |
| Keys in 1Password | No age key on disk. sops gets the identity from `SOPS_AGE_KEY_CMD` (`op read op://...`), so every sops run asks Windows Hello. `Unlock-Sops` loads it into one terminal for 15 minutes when you need many decryptions. |
| Codex sandbox | The `agent-guard` permission profile extends your own profile and denies key files, `.decrypted~*` copies, `*sopssecret*`, Talos `generated/`, `%APPDATA%\sops` and VS Code history and backups. The elevated Windows sandbox runs commands as `CodexSandbox*` users, and these denials become NTFS deny entries for them. `SOPS_AGE_*` variables are filtered out. Rules forbid `op`, `kubectl get secret`, `talosctl get mc` and age decryption, and ask before sops decrypts. |
| Claude Code | Claude Code has no sandbox on native Windows, so this layer matches text. Deny rules cover Read, Edit, Grep and Glob. `secret-guard.mjs` refuses commands that name those files, decrypt, print Secrets or Talos configs, or read recursively while such files exist. |
| Machine copy | `install.ps1 -Machine` writes the same policy where only administrators can change it: Claude `managed-settings.json` and Codex `requirements.toml`. Every Claude session warns when the machine copy is missing or older than the user copy. |

Claude Code cannot remove inherited environment variables. Never set
`SOPS_AGE_KEY` or `SOPS_AGE_KEY_FILE` in mise, direnv or the user environment,
and do not start an agent from a terminal where you ran `Unlock-Sops`.

## Install

Run in your own PowerShell, not from an agent:

```powershell
./agent-guard/install.ps1 -SopsKeyCommand 'op read op://<vault>/<item>/notesPlain'
```

Then, from an elevated PowerShell:

```powershell
./agent-guard/install.ps1 -Machine
```

Restart VS Code and any agent sessions afterwards. Rerun both after pulling
changes; `-Check` lists what differs without writing. The installer keeps
the first version of each file it changes as `<file>.agent-guard.bak`.

`-ProtectedRoot` (default `D:\git\homelab`) names the folders whose
repositories are protected. `-ExemptRoot` (default
`D:\git\homelab\infrastructure`) names repositories where agents may still
use local key files. This exception is temporary.

## Verify

```powershell
./agent-guard/verify.ps1          # real files and canaries, Codex only
./agent-guard/verify.ps1 -Claude  # also runs a headless Claude session (costs tokens)
```

Each probe prints only `DENIED` or `READABLE`. File contents never reach the
output.

## Daily use

- **Open a secret in VS Code.** The SOPS extension asks Windows Hello on open and again on save.
- **Many files at once** (diffs, `generate-talos.ps1`, re-keying): run `Unlock-Sops`, then `Lock-Sops` when done. The key also clears after 15 minutes.
- **Before asking an agent anything,** close decrypted editors. Agent extensions in the same VS Code window can see open documents.
- **An unexpected Windows Hello prompt** means something you did not start asked for the key. Deny it.

## Limits

- Claude's layer matches text, not file access. A script that opens files
  itself gets past it. It works because nothing readable holds a key.
- The Codex sandbox does not apply to commands you approve to run outside
  it. Those run as you.
- The Codex sandbox fails while the Codex desktop app's computer-use runtime
  (`node_repl.exe`) is running: "setup refresh had errors". Quit the app, or
  turn off computer use.
- kubectl and talosctl credentials are not covered yet. The rules only
  block the obvious commands.

## Development

```powershell
node --test 'agent-guard/*.test.mjs'
pwsh -File agent-guard/install.tests.ps1
```
