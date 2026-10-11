import { test } from 'node:test';
import assert from 'node:assert/strict';
import { evaluate, sessionWarnings } from './secret-guard.mjs';

const cwd = 'D:/git/homelab/cluster';
const bash = (command, dir = cwd) => ({ tool_name: 'Bash', tool_input: { command }, cwd: dir });
const ps = (command, dir = cwd) => ({ tool_name: 'PowerShell', tool_input: { command }, cwd: dir });
const read = (file_path) => ({ tool_name: 'Read', tool_input: { file_path } });
const edit = (file_path) => ({ tool_name: 'Edit', tool_input: { file_path } });
const clean = { scan: () => [] };
const dirty = { scan: () => ['D:/git/homelab/cluster/apps/x/.decrypted~s.sops.yaml'] };

const denied = [
  read('D:\\git\\homelab\\cluster\\age.agekey'),
  read('D:/git/homelab/cluster/clusters/main/kubernetes/app/.decrypted~secret.sops.yaml'),
  read('D:/git/homelab/cluster/clusters/main/kubernetes/flux-system/flux/sopssecret.secret.yaml'),
  read('C:/Users/x/AppData/Roaming/sops/age/keys.txt'),
  read('D:/git/homelab/cluster/clusters/main/talos/generated/main-k8s-control-1.yaml'),
  read('C:\\Users\\x\\AppData\\Roaming\\Code\\User\\History\\-1a2b\\entries.json'),
  read('C:\\Users\\x\\AppData\\Roaming\\Code\\Backups\\123\\file\\abc'),
  edit('C:/Users/x/.claude/settings.json'),
  edit('C:/Users/x/.claude/agent-guard/secret-guard.mjs'),
  { tool_name: 'Write', tool_input: { file_path: 'C:/Program Files/ClaudeCode/managed-settings.json' } },
  edit('C:/Users/x/.codex/managed_config.toml'),
  edit('C:/Users/x/.codex/rules/agent-guard.rules'),
  { tool_name: 'Glob', tool_input: { pattern: '**/*.agekey' } },
  { tool_name: 'Glob', tool_input: { pattern: '**/.decrypted~*' } },
  { tool_name: 'Grep', tool_input: { pattern: 'x', path: 'D:/git/homelab/cluster/clusters/main/talos/generated' } },
  bash('cat age.agekey'),
  bash('gci -Recurse | Get-Content ; cat ../media-cluster/age.agekey'),
  ps('Get-Content -Raw .\\clusters\\main\\kubernetes\\app\\.decrypted~x.sops.yaml'),
  bash('sops decrypt clusters/main/kubernetes/app/x.sops.yaml'),
  bash('sops -d x.sops.yaml'),
  bash('sops --decrypt --extract \'["data"]\' x.sops.yaml'),
  bash('sops x.sops.yaml'),
  bash('sops edit x.sops.yaml'),
  bash('sops set x.sops.yaml \'["stringData"]["A"]\' \'"b"\''),
  bash('sops exec-env x.sops.yaml env'),
  bash('sops updatekeys -y x.sops.yaml'),
  bash('sops rotate -i x.sops.yaml'),
  bash('git diff | cat && sops -d a.sops.yaml'),
  bash('echo "$(sops -d a.sops.yaml)"'),
  bash('SOPS_CONFIG=x sops -d a.sops.yaml'),
  bash('mise exec -- sops -d a.sops.yaml'),
  bash('/c/tools/sops.exe -d a.sops.yaml'),
  ps('& sops.exe -d x.sops.yaml'),
  ps('& "C:\\tools\\sops.exe" --decrypt x.sops.yaml'),
  ps('pwsh -NoProfile -Command "sops -d x.sops.yaml"'),
  bash('cmd /c sops -d x.sops.yaml'),
  bash('op read op://homelab/age/human'),
  bash('op item get abc --reveal'),
  bash('op document get sops-age --vault Homelab'),
  ps('op inject -i tpl.yaml'),
  ps('Unlock-Sops -Minutes 5'),
  bash('age -d -i key.txt file.age'),
  bash('age-keygen -o new.key'),
  bash('kubectl get secret -n flux-system sops-age -o yaml'),
  bash('kubectl -n flux-system get secrets'),
  bash('kubectl describe secret foo'),
  bash('kubectl config view --raw'),
  bash('talosctl get mc -n 192.168.10.201 -o yaml'),
  bash('talosctl -n 192.168.10.201 get machineconfig'),
  bash('talosctl read /system/state/config.yaml'),
  bash('talosctl gen secrets -o s.yaml'),
  bash('git -c diff.sopsdiffer.textconv="sops decrypt" diff'),
  bash('echo $SOPS_AGE_KEY_FILE'),
  ps('$env:SOPS_AGE_KEY_CMD'),
  ps('pwsh -File D:\\git\\homelab\\homelab-tools\\agent-guard\\install.ps1'),
  bash('sed -i s/deny/allow/ ~/.codex/managed_config.toml'),
];

const allowed = [
  read('D:/git/homelab/cluster/clusters/main/kubernetes/app/secret.sops.yaml'),
  read('D:/git/homelab/cluster/README.md'),
  read('C:/Users/x/.claude/settings.json'),
  read('D:/git/homelab/homelab-tools/agent-guard/install.ps1'),
  edit('D:/git/homelab/homelab-tools/agent-guard/secret-guard.mjs'),
  { tool_name: 'Grep', tool_input: { pattern: 'stringData', path: 'D:/git/homelab/cluster' } },
  { tool_name: 'Grep', tool_input: { pattern: 'sopssecret|agekey', path: 'D:/git/homelab/cluster' } },
  bash('git status && git diff'),
  bash('sops filestatus x.sops.yaml'),
  bash('sops encrypt -i new.sops.yaml'),
  bash('sops --version'),
  bash('printenv | grep -i sops'),
  bash('git log -S sops --oneline'),
  bash('kubectl get pods -A'),
  bash('kubectl get hr -A'),
  bash('talosctl health'),
  bash('talosctl get members'),
  bash('mise install'),
  bash('homelab-fmt --check'),
  bash('node --test agent-guard/secret-guard.test.mjs', 'D:/git/homelab/homelab-tools'),
  ps('pwsh -NoProfile -File agent-guard/install.tests.ps1', 'D:/git/homelab/homelab-tools'),
  bash('git log --oneline -- clusters/main/kubernetes/apps/operator/'),
  bash('rg -n "stringData" clusters'),
  ps('Set-Content x.toml "[p]`n`"~/AppData/Roaming/sops`" = `"deny`"`n"'),
];

for (const input of denied) {
  test(`denies ${input.tool_name} ${JSON.stringify(input.tool_input)}`, () => assert.ok(evaluate(input, clean)));
}
for (const input of allowed) {
  test(`allows ${input.tool_name} ${JSON.stringify(input.tool_input)}`, () => assert.equal(evaluate(input, clean), null));
}

const recursive = [
  bash('grep -r PASSWORD .'),
  bash('grep -rn token clusters/'),
  bash('rg --hidden --no-ignore CANARY'),
  bash('rg -uu CANARY'),
  ps('Get-ChildItem -Recurse -File | Get-Content'),
  ps('gci -r | sls CANARY'),
  ps('Select-String -Path .\\clusters\\* -Pattern x'),
  bash('findstr /s /i password *'),
  bash('find . -type f -exec cat {} +'),
  bash('find . -name "*.yaml" | xargs cat'),
  bash('tar czf /tmp/repo.tgz .'),
  bash('cp -r clusters /tmp/x'),
];

for (const input of recursive) {
  test(`pauses a recursive read while sensitive files exist: ${input.tool_input.command}`, () => {
    assert.match(evaluate(input, dirty) ?? '', /recursive reads are paused/);
    assert.equal(evaluate(input, clean), null);
  });
}

test('a recursive read scans the working directory and absolute paths it names', () => {
  const seen = [];
  evaluate(bash('grep -r x D:/git/homelab/media-cluster /d/git/homelab/shared-cluster', 'C:/work'), {
    scan: (roots) => {
      seen.push(...roots);
      return [];
    },
  });
  assert.deepEqual(seen.map((p) => p.replace(/\\/g, '/')), ['C:/work', 'D:/git/homelab/media-cluster', 'D:/git/homelab/shared-cluster']);
});

const infra = 'D:/git/homelab/infrastructure';
const exemptRoots = [infra];

test('an exempt repository keeps sops and its own key file', () => {
  for (const command of [
    'export SOPS_AGE_KEY_FILE="$PWD/.credentials/age/production.agekey" && sops -d secrets/drift-check.sops.json',
    'sops decrypt secrets/state-backup.sops.json',
  ]) {
    assert.equal(evaluate(bash(command, infra), { ...clean, exemptRoots }), null, command);
  }
});

test('an exempt repository does not cover paths outside it', () => {
  assert.ok(evaluate(bash('cat D:/git/homelab/cluster/age.agekey', infra), { ...clean, exemptRoots }));
  assert.ok(evaluate(bash('sops -d D:/git/homelab/cluster/clusters/main/clusterenv.yaml', infra), { ...clean, exemptRoots }));
  assert.ok(evaluate(bash('sops -d secrets/x.sops.json', cwd), { ...clean, exemptRoots }));
});

test('an exempt repository still refuses everything else', () => {
  for (const input of [
    read(`${infra}/.credentials/age/production.agekey`),
    bash('cat .decrypted~x.sops.json', infra),
    bash('op read op://v/i/f', infra),
    bash('kubectl get secrets -A', infra),
  ]) {
    assert.ok(evaluate(input, { ...clean, exemptRoots }), JSON.stringify(input.tool_input));
  }
});

test('the scan ignores key files in exempt repositories only', async () => {
  const { mkdtempSync, mkdirSync, writeFileSync, rmSync } = await import('node:fs');
  const { tmpdir } = await import('node:os');
  const { join } = await import('node:path');
  const { sensitiveFilesUnder } = await import('./secret-guard.mjs');
  const root = mkdtempSync(join(tmpdir(), 'agent-guard-'));
  try {
    for (const dir of ['infra/.credentials', 'cluster/talos/generated', 'cluster/apps']) mkdirSync(join(root, dir), { recursive: true });
    writeFileSync(join(root, 'infra/.credentials/production.agekey'), 'dummy');
    const exempt = { exemptRoots: [join(root, 'infra')] };
    assert.deepEqual(sensitiveFilesUnder([join(root, 'infra')], exempt), []);
    writeFileSync(join(root, 'infra/.decrypted~x.sops.json'), 'dummy');
    assert.equal(sensitiveFilesUnder([join(root, 'infra')], exempt).length, 1);
    assert.equal(sensitiveFilesUnder([join(root, 'cluster')], exempt).length, 1);
    writeFileSync(join(root, 'cluster/apps/age.agekey'), 'dummy');
    assert.equal(sensitiveFilesUnder([join(root, 'cluster')], exempt).length, 2);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('a non-recursive command never scans', () => {
  evaluate(bash('ls -la'), { scan: () => assert.fail('scanned') });
});

const machine = 'C:\\pf\\ClaudeCode\\agent-guard\\secret-guard.mjs';
const base = {
  env: { APPDATA: 'C:\\Users\\x\\AppData\\Roaming', ProgramFiles: 'C:\\pf' },
  exists: () => false,
  home: 'C:\\Users\\x',
  self: 'C:\\Users\\x\\.claude\\agent-guard\\secret-guard.mjs',
  platform: 'win32',
};
const versions = (mine, theirs) => (p) => {
  const v = p === machine ? theirs : mine;
  if (v === null) throw new Error('missing');
  return `// agent-guard version: ${v}\n`;
};

test('session warnings report key variables and key files', () => {
  const warnings = sessionWarnings({
    ...base,
    env: { ...base.env, SOPS_AGE_KEY_FILE: 'D:/x/age.agekey' },
    exists: (p) => p.endsWith('keys.txt'),
    read: versions('abc', 'abc'),
  });
  assert.equal(warnings.length, 3);
  assert.match(warnings[0], /^SOPS_AGE_KEY_FILE is set/);
  assert.match(warnings[1], /keys\.txt/);
});

test('session warnings report a missing or stale machine copy', () => {
  assert.match(sessionWarnings({ ...base, read: versions('abc', null) })[0], /machine copy is missing/);
  assert.match(sessionWarnings({ ...base, read: versions('abc', 'old') })[0], /machine copy is old and the user copy is abc/);
});

test('session warnings are empty on a clean, current machine', () => {
  assert.deepEqual(sessionWarnings({ ...base, read: versions('abc', 'abc') }), []);
});

test('the machine copy does not compare itself with itself', () => {
  assert.deepEqual(sessionWarnings({ ...base, self: machine, read: versions('abc', 'abc') }), []);
});
