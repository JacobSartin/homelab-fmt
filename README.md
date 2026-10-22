# homelab-fmt

`homelab-fmt` keeps the YAML in the homelab GitOps repositories in a
consistent key order and blank-line layout. It is a single static binary with
no runtime. Indentation, wrapping and quoting are left to
[oxfmt](https://oxc.rs/docs/guide/usage/formatter) in the editor.

The order comes from [templates](internal/templates/bundled): example
documents laid out the way formatted files should look.

| Template | Formats |
| --- | --- |
| [`app-template.yaml`](internal/templates/bundled/app-template.yaml) | bjw-s app-template values |
| [`kubernetes.yaml`](internal/templates/bundled/kubernetes.yaml) | top-level fields and `metadata` of every Kubernetes object |
| [`helm-release.yaml`](internal/templates/bundled/helm-release.yaml) | Flux `HelmRelease` |
| [`flux-kustomization.yaml`](internal/templates/bundled/flux-kustomization.yaml) | Flux `Kustomization` |
| [`prometheus-rule.yaml`](internal/templates/bundled/prometheus-rule.yaml) | prometheus-operator `PrometheusRule`, with a blank line between rules |
| [`kustomization.yaml`](internal/templates/bundled/kustomization.yaml), [`kustomize-component.yaml`](internal/templates/bundled/kustomize-component.yaml) | kustomize `Kustomization` and `Component` |

How it edits:

- It moves whole lines, so scalars, quoting, indentation and comments are
  never re-printed. Output that oxfmt accepted stays accepted.
- Comments directly above a key move with it; blank lines and detached
  comments stay where they were. The comment at the top of a document (such
  as the schema modeline) stays on top.
- A mapping stays unsorted, with a warning, when sorting would move a YAML
  alias above its anchor.
- The result must parse to the same data as the input, or the file is
  reported as an error and left unchanged.

## Usage

```sh
homelab-fmt                      # format every YAML file Git does not ignore
homelab-fmt --check              # list files that need formatting
homelab-fmt a.yaml b.md          # format only these; non-YAML paths are skipped
homelab-fmt apps/                # recursively format YAML in a directory
homelab-fmt .                    # explicit recursive search, also outside Git
homelab-fmt explain a.yaml       # show the template and rule for each mapping
homelab-fmt explain apps/        # explain YAML files in a directory
```

Without paths, Git must be available and the current directory must be in a
Git worktree. Outside Git, pass explicit files or directories. Directory
arguments inside Git respect ignored paths and nested repository boundaries;
explicit files can be ignored by Git. Searches never follow symbolic links.
Overlapping arguments are deduplicated and processed in path order. Missing
paths and discovery failures return exit status 3.

Runs without paths summarize changed files, reordered mappings, blank lines
added or removed, warnings and errors. Check mode reports the edits it would
make; write mode counts only successful files. Rejected edits do not count.

In `explain` paths, dots separate keys. Within a key, `~0` represents a tilde,
`~1` a literal dot, and `~2` an empty key; `(root)` is the document root.

| Exit status | Meaning |
| --- | --- |
| 0 | Done; with `--check`, nothing needs formatting |
| 1 | `--check` found files that need formatting |
| 2 | Usage error: unknown flag, or `explain` without paths |
| 3 | Runtime error: a file could not be listed, read or written, is not valid YAML, or would change meaning. Wins over 1. |

## Templates

A template is one YAML document. Its values are placeholders; only its keys,
blank lines and `homelab-fmt` comments matter.

- **Order.** Keys are ordered as in the template. Keys it does not list keep
  their relative order after the listed ones.
- **Wildcards.** A `"*"` key stands for any key, such as each controller under
  `controllers`. A named key next to it wins for that name. In a list, the
  first item describes every item. Put `"*"` last in its mapping.
- **Wildcard examples.** Keys starting with `*`, such as `"*nfs"` and
  `"*emptyDir"`, are wildcards too. Use several when one example cannot stay
  valid against the schema, as with app-template persistence types. Their
  orders are merged: a key from a later example goes right before the next key
  the examples share.
- **Blank lines.** A blank line between two keys requires a blank line after
  the first one. A missing blank line requires nothing. Keys the template does
  not list are separated from the listed ones by a blank line, unless the
  mapping is `compact`.
- **Directives** go in a comment on a key's line and apply to the mapping
  under that key:
  - `# homelab-fmt: sort` sorts the keys the template does not list.
  - `# homelab-fmt: compact` removes blank lines between the entries.
  - `# homelab-fmt: separate` requires a blank line between the entries.

  On a key holding a list, `compact` and `separate` apply to the list's
  items instead, with the comments above each dash kept with its item.

  Several directives can share a comment, separated by commas.

Header comments, above the first key, select documents and layer templates:

- `# homelab-fmt: schema <glob>` matches documents whose
  `yaml-language-server` `$schema` URL matches the glob, where `*` matches
  anything.
- Without `schema`, the template's `apiVersion` group and `kind` select
  documents. The version is ignored, and `"*"` matches any value.
- Each document uses the single most specific template: `schema`, then group
  and kind, then templates with wildcards.
- `# homelab-fmt: extends <template>` adds another template's rules. For a
  mapping both describe, the child's keys come first, followed by the parent's
  keys the child does not list.

To change the order, edit the template. To support a new kind, add a template,
usually extending `kubernetes.yaml`. Keep templates valid against their
schema, with a `yaml-language-server` modeline where one exists. Tests check
that every template loads, selects itself, formats to itself and is restored
after its mappings are shuffled.

## Installing in a consumer repository

Releases publish binaries for Linux, macOS and Windows with build provenance
attestations, which mise verifies. Pin a release in the repository's
`mise.toml`:

```toml
[tools]
"github:JacobSartin/homelab-fmt" = "0.1.0"
```

To format on save in VS Code, keep oxfmt as the YAML formatter and run
`homelab-fmt` after each save with the
[Run on Save](https://marketplace.visualstudio.com/items?itemName=emeraldwalk.RunOnSave)
extension. VS Code must see mise's shims on `PATH`.

```json
"emeraldwalk.runonsave": {
  "commands": [{ "match": "\\.ya?ml$", "cmd": "homelab-fmt \"${file}\"" }]
}
```

## Layout

- [`cmd/homelab-fmt`](cmd/homelab-fmt): command-line interface.
- [`internal/templates`](internal/templates): loads the bundled templates,
  compiles them to rules, layers `extends` chains and picks one per document.
- [`internal/format`](internal/format): `Format` and `Explain`; fixture pairs
  in `testdata/<name>.in.yaml` / `<name>.out.yaml`.
- [`internal/rules`](internal/rules): the rule model (path, order, spacing),
  path matching and natural sort.
- [`internal/yamltext`](internal/yamltext): block mappings as line ranges.
- [`internal/files`](internal/files): discovery through `git ls-files` and
  explicit-path selection.

## Development

```sh
go test ./...
go run ./cmd/homelab-fmt --check   # from a consumer repository root
```

Pushing a `v*` tag runs [the release workflow](.github/workflows/release.yaml),
which tests, builds with GoReleaser and attests the archives.
