#!/usr/bin/env node
// Claude Code hook that keeps agents away from SOPS keys and decrypted secrets.
//
//   PreToolUse:   refuse tool calls that name a key file, a decrypted SOPS copy,
//                 generated Talos configs or VS Code history; refuse edits to
//                 the installed guard; refuse commands that decrypt or print
//                 secrets; refuse recursive reads while sensitive files exist
//                 under the searched tree.
//   SessionStart: (--session-start) warn when a plaintext key or key variable
//                 is present, or when the machine copy is missing or stale.
//
// Matching is textual, so it stops accidents and obvious attempts. The
// guarantee comes from keys being absent from disk and from the environment,
// and from every decryption needing the owner's approval in 1Password.
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

// agent-guard version: dev

// Repositories where agents may still use local key files, a temporary
// exception. install.ps1 fills this in.
const EXEMPT_ROOTS = [];

const KEY_FILE = /\.agekey\b/i;
const SENSITIVE_PATHS = [
  [KEY_FILE, 'an age private key file'],
  [/(^|[\\/\s'"])\.decrypted~/i, 'a decrypted SOPS copy'],
  [/sopssecret/i, 'the plaintext Flux key Secret'],
  [/sops[\\/]+age[\\/]+keys\.txt/i, 'the default SOPS age key file'],
  [/talos[\\/]+generated([\\/]|\b)/i, 'generated Talos machine configs'],
  [/Code[\\/]+(User[\\/]+History|Backups)([\\/]|\b)/i, 'VS Code history or backups'],
];

// Installed guard configuration: readable, but agents may not change it. The
// source in homelab-tools stays editable.
const GUARD_CONFIG = /[\\/]\.claude[\\/]+(settings[^\\/]*\.json|agent-guard)|ClaudeCode[\\/]|managed-settings|[\\/]\.codex[\\/]+(config\.toml|managed_config\.toml|rules)|OpenAI[\\/]+Codex/i;
const WRITE_TOOLS = new Set(['Edit', 'Write', 'MultiEdit', 'NotebookEdit']);
// Only the owner reinstalls the guard; its tests run against a scratch root.
const INSTALLER_RUN = /agent-guard[\\/]+install\.ps1/i;

// File names that make a recursive read dangerous while they exist.
const SENSITIVE_NAME = /^\.decrypted~|\.agekey$|sopssecret/i;
const SKIP_DIRS = new Set(['node_modules', '.git', '.npm-cache', 'dist', '.validation', '.venv', '.mise', '.terraform']);

// sops subcommands and flags that need no private key.
const SOPS_SAFE = new Set(['filestatus', 'encrypt', '-e', '--encrypt', '--version', '-v', '--help', '-h', 'help']);
const SOPS_UNSAFE = /^(-d|--decrypt|decrypt|edit|exec-env|exec-file|set|unset|rotate|-r|updatekeys|publish|keyservice|groups)$/i;

const COMMAND_RULES = [
  [/(^|[\s;&|(`'"])op(\.exe)?\s+(read|inject|run|item|document|signin|account|whoami|vault|connect|service-account|user|plugin|events-api)\b/i,
    'the 1Password CLI is reserved for the owner'],
  [/\bUnlock-Sops\b/i, 'Unlock-Sops is reserved for the owner'],
  [/\bage(\.exe)?\s[^\n;&|]*(^|\s)(-d|--decrypt|-i|--identity)\b/i, 'age decryption is reserved for the owner'],
  [/\bage-keygen\b/i, 'keys must be generated outside agent sessions'],
  [/\bage-plugin-[a-z0-9-]+/i, 'age plugins are reserved for the owner'],
  [/\bkubectl(\.exe)?\b[^\n;&|]*\bsecrets?\b/i, 'reading Kubernetes Secrets is reserved for the owner'],
  [/\bkubectl(\.exe)?\b[^\n;&|]*\bconfig\s+view\b[^\n;&|]*--raw\b/i, 'kubeconfig credentials are reserved for the owner'],
  [/\btalosctl(\.exe)?\b[^\n;&|]*\b(get\s+(mc|machineconfigs?)\b|read\b|cat\b|gen\s+(secrets|config)\b|config\s+new\b|rotate-ca\b)/i,
    'Talos machine configs and secrets are reserved for the owner'],
  [/diff\.sopsdiffer|\btextconv\b/i, 'the SOPS diff driver is owner-only'],
  [/SOPS_AGE_(KEY|SSH|RECIPIENTS_CMD)/i, 'SOPS key variables are reserved for the owner'],
];

// Commands that read every file under a directory.
const RECURSIVE_READS = [
  /\b[ef]?grep\b[^\n;&|]*\s-[a-zA-Z]*[rR]/,
  /\bgrep\b[^\n;&|]*--(dereference-)?recursive\b/,
  /\brg(\.exe)?\b[^\n;&|]*\s(-[a-zA-Z]*u|--hidden|--no-ignore\S*|-\.)(\s|$)/,
  /\bfindstr\b[^\n;&|]*\s\/s\b/i,
  /-Recurse\b/i,
  /\b(ls|dir|gci|Get-ChildItem|find)\b[^\n;&]*\|[^\n;&]*\b(cat|type|gc|Get-Content|sls|Select-String|xargs|ForEach-Object|%)\b/i,
  /\bfind\b[^\n;&|]*\s-exec\b/,
  /\b(Select-String|sls)\b[^\n;&|]*\*/i,
  /\b(tar|zip|7z|Compress-Archive|robocopy|xcopy)\b/i,
  /\b(cp|Copy-Item)\b[^\n;&|]*\s-[a-zA-Z]*[rR]/,
];

const inside = (path, root) => {
  const p = resolve(path).toLowerCase();
  const r = resolve(root).toLowerCase();
  return p === r || p.startsWith(r.endsWith(sep) ? r : r + sep);
};

export function sensitiveFilesUnder(roots, { limit = 5, exemptRoots = EXEMPT_ROOTS } = {}) {
  const hits = [];
  const keyExempt = (full) => KEY_FILE.test(full) && exemptRoots.some((root) => inside(full, root));
  const walk = (dir, depth) => {
    if (depth > 14 || hits.length >= limit) return;
    let entries;
    try {
      entries = readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      const full = join(dir, e.name);
      if (e.isDirectory()) {
        if (SKIP_DIRS.has(e.name)) continue;
        if (e.name === 'generated' && /[\\/]talos$/i.test(dir)) hits.push(full);
        else walk(full, depth + 1);
      } else if (SENSITIVE_NAME.test(e.name) && !keyExempt(full)) {
        hits.push(full);
      }
    }
  };
  for (const root of roots) if (existsSync(root)) walk(root, 0);
  return hits;
}

// The program a simple command runs, after env assignments, call operators,
// runners such as `mise exec --` and shell wrappers such as `pwsh -Command`.
function commandWords(segment) {
  let words = segment.trim().split(/\s+/).filter(Boolean).map((w) => w.replace(/^['"]+|['"]+$/g, ''));
  for (let guard = 0; words.length && guard < 10; guard++) {
    const w = words[0];
    if (/^[A-Za-z_][A-Za-z0-9_]*=/.test(w) || /^(&|\.|sudo|env|time|nice|nohup|exec|command|call|Invoke-Expression|iex)$/i.test(w)) {
      words = words.slice(1);
    } else if (/^(mise|npx|pnpm|bunx)(\.exe)?$/i.test(w)) {
      const dashes = words.indexOf('--');
      words = dashes >= 0 ? words.slice(dashes + 1) : words.slice(1);
    } else if (/^(pwsh|powershell|bash|sh|zsh|cmd)(\.exe)?$/i.test(w)) {
      words = words.slice(1);
      while (words.length && /^[-/]/.test(words[0])) words = words.slice(1);
    } else {
      break;
    }
  }
  return words;
}

function sopsVerdict(command, tool) {
  // A backtick substitutes a command in Bash but escapes a character in PowerShell.
  const separators = tool === 'PowerShell' ? /[\n;&|(){}]+/ : /[\n;&|()`{}]+/;
  for (const segment of command.split(separators)) {
    const words = commandWords(segment);
    if (!words.length || !/(^|[\\/])sops(\.exe)?$/i.test(words[0])) continue;
    const args = words.slice(1);
    if (args.some((a) => SOPS_UNSAFE.test(a))) return 'sops decryption is reserved for the owner';
    // Bare `sops <file>` opens an editor on the decrypted file.
    if (!args.some((a) => SOPS_SAFE.has(a.toLowerCase()))) return 'sops needs the private key for this';
  }
  return null;
}

// Absolute paths named in a command, plus the working directory.
function searchRoots(command, cwd) {
  const roots = new Set();
  if (cwd) roots.add(resolve(cwd));
  for (const m of command.matchAll(/(?:^|[\s'"=])((?:[A-Za-z]:|\/[a-z](?=\/))[\\/][^\s'"|;&]*)/g)) {
    const p = m[1].replace(/^\/([a-z])\//, (_, d) => `${d.toUpperCase()}:/`).replace(/[*?].*$/, '');
    if (isAbsolute(p)) roots.add(resolve(p));
  }
  return [...roots];
}

function pathsOf(tool, ti) {
  if (tool === 'Grep') return [ti.path, ti.glob];
  if (tool === 'Glob') return [ti.path, ti.pattern];
  return [ti.file_path, ti.notebook_path, ti.path];
}

export function evaluate(input, { scan = sensitiveFilesUnder, exemptRoots = EXEMPT_ROOTS } = {}) {
  const tool = input?.tool_name ?? '';
  const ti = input?.tool_input ?? {};
  for (const value of pathsOf(tool, ti)) {
    if (typeof value !== 'string') continue;
    for (const [re, what] of SENSITIVE_PATHS) if (re.test(value)) return `refused to touch ${what}`;
    if (WRITE_TOOLS.has(tool) && GUARD_CONFIG.test(value)) return 'refused to change the installed agent guard';
  }
  if (typeof ti.command !== 'string') return null;
  const cmd = ti.command;
  const roots = searchRoots(cmd, input.cwd);
  // In an exempt repository, sops and its local key file stay usable as long
  // as the command stays inside exempt repositories.
  const exempt = Boolean(input.cwd) && roots.every((root) => exemptRoots.some((e) => inside(root, e)));
  for (const [re, what] of SENSITIVE_PATHS) {
    if (exempt && re === KEY_FILE) continue;
    if (re.test(cmd)) return `the command references ${what}`;
  }
  if (GUARD_CONFIG.test(cmd) || INSTALLER_RUN.test(cmd)) return 'the command references the installed agent guard';
  if (!exempt) {
    const sops = sopsVerdict(cmd, tool);
    if (sops) return sops;
  }
  for (const [re, why] of COMMAND_RULES) {
    if (exempt && /SOPS_AGE/.test(re.source)) continue;
    if (re.test(cmd)) return why;
  }
  if (RECURSIVE_READS.some((re) => re.test(cmd))) {
    const hits = scan(roots, { exemptRoots });
    if (hits.length) {
      return `recursive reads are paused while sensitive files exist under the searched tree (${hits.length}+, e.g. ${hits[0]}). Use rg, git grep or the Grep tool, which skip ignored files, or name the files to read`;
    }
  }
  return null;
}

const versionOf = (text) => /agent-guard version: (\S+)/.exec(text)?.[1] ?? null;

// SessionStart: report drift and plaintext key material.
export function sessionWarnings({
  env = process.env,
  exists = existsSync,
  read = (p) => readFileSync(p, 'utf8'),
  home = homedir(),
  self = fileURLToPath(import.meta.url),
  platform = process.platform,
} = {}) {
  const warnings = [];
  for (const name of ['SOPS_AGE_KEY', 'SOPS_AGE_KEY_FILE', 'SOPS_AGE_SSH_PRIVATE_KEY_FILE']) {
    if (env[name]) warnings.push(`${name} is set in this session's environment.`);
  }
  const appData = env.APPDATA ?? join(home, 'AppData', 'Roaming');
  for (const p of [join(appData, 'sops', 'age', 'keys.txt'), join(home, '.config', 'sops', 'age', 'keys.txt')]) {
    if (exists(p)) warnings.push(`A plaintext SOPS key file exists at ${p}.`);
  }
  if (platform === 'win32') {
    const machine = join(env.ProgramFiles ?? 'C:\\Program Files', 'ClaudeCode', 'agent-guard', 'secret-guard.mjs');
    if (resolve(dirname(self)) !== resolve(dirname(machine))) {
      let mine = null;
      let theirs = null;
      try {
        mine = versionOf(read(self));
      } catch {}
      try {
        theirs = versionOf(read(machine));
      } catch {}
      if (!theirs) warnings.push('The machine copy is missing; run install.ps1 -Machine from an elevated PowerShell.');
      else if (mine !== theirs) warnings.push(`The machine copy is ${theirs} and the user copy is ${mine}; rerun install.ps1 -Machine.`);
    }
  }
  return warnings;
}

function main() {
  if (process.argv.includes('--session-start')) {
    const warnings = sessionWarnings();
    if (warnings.length) process.stdout.write(JSON.stringify({ systemMessage: `agent-guard: ${warnings.join(' ')}` }));
    return;
  }
  let input;
  try {
    input = JSON.parse(readFileSync(0, 'utf8'));
  } catch {
    input = null;
  }
  const reason = input ? evaluate(input) : 'unreadable hook input, failing closed';
  if (reason) {
    process.stdout.write(JSON.stringify({
      hookSpecificOutput: {
        hookEventName: 'PreToolUse',
        permissionDecision: 'deny',
        permissionDecisionReason: `agent-guard: ${reason}. If the owner needs this, they run it in their own terminal.`,
      },
    }));
  }
}

if (process.argv[1] && resolve(fileURLToPath(import.meta.url)) === resolve(process.argv[1])) main();
