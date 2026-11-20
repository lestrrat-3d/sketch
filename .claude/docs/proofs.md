# Lean proofs

`_proofs/` holds machine-checked Lean 4 proofs about the sketch engine, plus a Go checker that fails when a
covered Go function changes without its theorem being re-read. The proofs are about Lean models of Go functions
under the assumptions table below. They are not proofs about compiled Go.

Only the infrastructure exists so far. `_proofs/Sketch/Smoke.lean` is a placeholder theorem that exercises the
build; `_proofs/trace.json` has no entries. The first real proof deletes the placeholder.

| Question | Section |
|---|---|
| Where files live | Directory layout |
| Lean/Mathlib pins, options, file conventions | Lean project |
| What the proofs assume and do NOT prove | Assumptions |
| Manifest format, hash, checker | Trace manifest, Checker |
| CI workflow | CI workflow |
| Adding a theorem for a new Go function | Recipe: cover a new Go function |
| Upgrading Lean or Mathlib | Upgrade Lean and Mathlib |
| Running everything locally | Run locally |

## Directory layout

```
_proofs/
  lakefile.toml          Lake config: package, lean_lib Sketch, leanOptions, Mathlib require
  lean-toolchain         leanprover/lean4:v4.36.0, equal to Mathlib's own at the pinned commit
  lake-manifest.json     exact commit of Mathlib and every transitive Lean package
  Sketch.lean            root module; `lake exe mk_all` writes it from every Sketch/**.lean
  Sketch/Smoke.lean      placeholder theorem; delete with the first real proof
  go.mod                 module github.com/lestrrat-3d/sketch/_proofs; standard library only
  trace.json             theorem to Go function manifest
  tracecheck/            package main: manifest checker and its tests
```

- New area → new directory `Sketch/<Area>/`. One topic per file.
- `_proofs` starts with `_`, so the root module, `go vet ./...`, `golangci-lint` and `go list ./...` never see it.
  `.lake/` starts with `.`, so `go list ./...` inside `_proofs` skips it too.
- `.gitignore` covers `/_proofs/.lake/`. NEVER commit `.lake/`. A checkout that builds the proofs holds about
  7 GB there; remove `_proofs/.lake` when done.
- `_proofs/` is a nested Go module, so root `go mod tidy` ignores it. Run `go mod tidy` in `_proofs` after
  adding a Go import there (none today).

## Lean project

### Pinning

- `lean-toolchain` MUST equal `.lake/packages/mathlib/lean-toolchain`.
- `lakefile.toml` requires Mathlib by tag. `lake-manifest.json` pins the commit. `lake build` never updates
  the manifest.

### Build is warning-clean

- ALWAYS build with `lake build --wfail`. CI passes `--wfail` too. It fails on every warning, including
  `declaration uses sorry`.
- Fix a deprecation by using the replacement the warning names. NEVER silence it.
- Remove an unused hypothesis. NEVER prefix it with `_`.
- NEVER add `set_option ... false` to a file to silence a linter. A linter that is wrong for this project is
  disabled once in `lakefile.toml` with a comment. `lakefile.toml` disables `linter.style.header` (Mathlib's
  copyright block rule) and `linter.style.longLine` (Mathlib's 100-column rule).

### File conventions

Every file:

```lean
module

public import Mathlib            -- or public import Sketch.<...>

/-!
# <Title>

<What this file models, one paragraph. Name the Go functions by package-qualified name.>
-/

public section

namespace Sketch.<Area>
...
end Sketch.<Area>

end
```

- `module` + `public import` + `public section` are required. Without them Lake warns that `Mathlib` is
  designed for the module system, and `--wfail` fails on it.
- A model definition another file unfolds MUST be `@[expose]`.
- The `/-! # Title -/` docstring is required by Mathlib's header linter.
- NEVER write `axiom`, `sorry`, `admit` or `native_decide`. CI's axiom audit fails on all four.
- Every assumption is a hypothesis of a theorem or a field of a model structure.
- `lake exe mk_all` rewrites `Sketch.lean` with the header `module  -- shake: keep-all --deprecated_module: ignore`.
  Keep that line as it writes it; `lake exe mk_all --check` compares against it.

### Naming and docstrings

- Defs are `lowerCamelCase`; theorems are `snake_case` embedding def names. A Lean def mirroring Go
  `fooBar` is `fooBar`.
- A def mirroring a Go function has a docstring ``Model of `pkg.Func`.`` plus any deliberate difference.
- A top-level theorem docstring states the guarantee in words, the Go functions it covers by package-qualified
  name, and the assumption ids its hypotheses carry. File paths and hashes live only in `trace.json`.

## Assumptions

The proofs do NOT prove the rows below. A change that breaks a row can make a proved bound false while every
theorem still builds. The table is empty until the first theorem lands.

| Id | Assumption | Lean form | Go evidence | Covered Go entries |
|---|---|---|---|---|

- Adding a row = new id `A<n+1>`. NEVER renumber. Delete a row only when no manifest entry cites it.
- The checker fails when a manifest entry cites an id with no row starting `| A<n> |` in this file.

## Trace manifest

`_proofs/trace.json`. Every sketch function on a proved path is an entry: the function a theorem models, and
each sketch function it calls whose behavior the proof relies on (as `assumes`). Standard-library calls are
covered by an assumption row, not an entry.

```json
{
  "entries": [
    {
      "file": "geom/example.go",
      "func": "Example",
      "role": "models",
      "theorems": ["Sketch/Area/Topic.lean:Sketch.Area.example_sound"],
      "assumptions": ["A1"],
      "sha256": "<64 hex>"
    }
  ]
}
```

| Field | Rule |
|---|---|
| `file` | Repo-relative path. Key is (`file`, `func`), so same-named functions in build-tag twins are separate entries. |
| `func` | `Name` for a function, `Type.Name` for a method (receiver type without `*`). |
| `role` | `models`: a Lean def mirrors it; needs at least 1 theorem. `assumes`: an assumption row states its behavior; needs at least 1 assumption. |
| `theorems` | `<file under _proofs>:<fully qualified name>`. |
| `assumptions` | Assumption ids. |
| `sha256` | See Hash definition. |

- Entries are sorted by (`file`, `func`) and have no duplicate keys. The file is pretty-printed with a 2-space
  indent and a trailing newline.
- Each covered function's doc comment ends with exactly this line:

```go
// Lean proofs depend on this function; see _proofs/trace.json.
```

- An undocumented covered function gets one line starting with its name that states what the code does.

## Checker

`_proofs/tracecheck` is a standard-library-only `package main`. Its tests use `testing` without testify, so
the module has no requirements and no `go.sum`.

### Hash definition

`sha256` of the exact source bytes from the `func` keyword through the closing `}`:
`src[fset.Position(fd.Pos()).Offset : fset.Position(fd.End()).Offset]` for the `*ast.FuncDecl`.

- It includes the receiver, signature, body and comments inside the body.
- It excludes the doc comment, so editing the pointer line or the doc text never changes the hash.
- It is whitespace-sensitive. Repo Go is gofmt'd, so this only matters on real edits.
- `go/parser` ignores build tags, so both files of a build-tag twin parse.

### Checks

All must pass. Each failure is one line naming file, func and the next action.

| # | Check | Failure line starts with |
|---|---|---|
| 1 | Manifest parses; fields valid; sorted; no duplicate keys | `<file> <func>: ...` |
| 2 | `file` exists; `func` found exactly once; hash matches | `<file> <func>: source changed; re-check <theorems> ...` |
| 3 | Each theorem's Lean file exists and has a line matching `^\s*(theorem\|lemma)\s+<last name component>\b` | `<file> <func>: theorem <name> not declared in _proofs/<lean file>` |
| 4 | Each assumption id has a row starting `\| A<n> \|` in this file | `<file> <func>: assumption A<n> has no row` |
| 5 | Each entry's Go doc comment contains `_proofs/trace.json` | `<file> <func>: doc comment lacks the pointer` |
| 6 | Reverse: every function in a repo `.go` file (skipping `_`/`.`-prefixed directories and `_test.go`) whose doc comment contains `_proofs/trace.json` is an entry | `<file> <func>: doc comment points to ... but the manifest has no entry` |
| 7 | Each entry's `file` appears single-quoted in `.github/workflows/proofs.yml` | `<file> <func>: '<file>' is missing from .github/workflows/proofs.yml` |

### CLI

Run from `_proofs`:

- `go run ./tracecheck` runs the checks and exits 0 or 1. `-repo <dir>` sets the repository root (default `..`).
- `go run ./tracecheck -update` rewrites the `sha256` of every existing entry whose function is found, prints
  each change, then runs every check. It NEVER adds or removes entries.
- Run `-update` ONLY after re-reading each affected theorem against the changed Go code. The PR diff of
  `trace.json` shows reviewers which theorems the change touched.

## CI workflow

`.github/workflows/proofs.yml` is separate from `ci.yml`, which does not run it. It is NOT a required check: a
path-filtered required check stays pending on PRs it skips. **NEVER merge a PR whose `Proofs` workflow failed**,
even when the main CI is green.

- Triggers: `pull_request` and `push` to `main` for the `paths` list (`_proofs/**`, the workflow, this file and
  each covered Go file), and `workflow_dispatch`. The `paths` list is a YAML anchor shared by both triggers.
  Check 7 reads the file as text, so a duplicated list passes as well.
- Job `trace` (`proofs trace`, 5-minute cap): checkout, `actions/setup-go` with `go-version-file: _proofs/go.mod`,
  then `go vet ./...`, `go test ./...` and `go run ./tracecheck` in `_proofs`.
- Job `lean` (`proofs build`, 30-minute cap): checkout, then `leanprover/lean-action` pinned by full commit SHA
  with the version in a trailing comment, as every action in this repository is. Dependabot's `github-actions`
  entry updates the pin.

| lean-action input | Value | Reason |
|---|---|---|
| `lake-package-directory` | `_proofs` | The Lake project is not at the repository root. |
| `use-github-cache` | `false` | A multi-GB `.lake` entry would evict `ci.yml`'s Go build caches. `lake exe cache get` downloads Mathlib's artifacts from Mathlib's servers instead. |
| `use-mathlib-cache` | `true` | Runs `lake exe cache get`. |
| `mk_all-check` | `true` | Fails when a `Sketch/**.lean` file is not imported by `Sketch.lean`, so `lake build` cannot skip it. |
| `build`, `build-args` | `true`, `--wfail` | Warnings fail the build. |
| `axiom-audit`, `axiom-audit-root` | `true`, `Sketch` | Fails when a `Sketch.*` declaration depends on an axiom outside `propext`, `Classical.choice`, `Quot.sound`: catches `sorry`, `admit`, `native_decide` and any `axiom`. |
| `test`, `lint`, `leanchecker` | `false` | No Lake test or lint driver is configured. `leanchecker` replays declarations the kernel already checked. |

lean-action runs elan's `elan-init.sh` from elan's `master` branch, which the action SHA does not pin. The Lean
version is pinned by `lean-toolchain`.

| Failure | Meaning | Action |
|---|---|---|
| `trace` red, "source changed" | A covered Go function changed | Re-check the theorem; fix the proof or the code; then `-update` |
| `trace` red, workflow path missing | A new entry's file is not in `paths` | Add it to the `paths` list in `proofs.yml` |
| `lean` red at build | Proof broken or new warning | Fix the Lean file |
| `lean` red at mk_all | New file not imported | `lake exe mk_all` in `_proofs`, commit `Sketch.lean` |
| `lean` red at axiom-audit | `sorry`/`axiom`/`native_decide` reached a `Sketch` declaration | Remove it |
| `lean` red at cache get | Mathlib cache server down, or artifacts for the pin no longer served | Rerun; if the artifacts are gone, upgrade Mathlib |

A workflow edit needs a token with the `workflow` scope to merge.

## Recipe: cover a new Go function

1. Read the Go function. List every sketch function it calls on the path the guarantee depends on.
2. Write an `@[expose]` Lean def mirroring its control flow in `Sketch/<Area>/<Topic>.lean`. Use ℚ/ℤ for exact
   arithmetic. A fact Lean cannot see (hardware, standard library, a callee you do not model) becomes a
   hypothesis or a model-structure field, plus a new assumption row. NEVER an `axiom`.
3. Prove the end-to-end theorem stating the published guarantee about the def. Write its docstring per the
   naming rules above.
4. `lake exe mk_all` (updates `Sketch.lean`), then `lake build --wfail`.
5. Add `trace.json` entries: `models` for the function, `assumes` for each relied-on callee. Add the pointer
   line to each. Put placeholder hashes of 64 zeros, run `go run ./tracecheck -update`, then
   `go run ./tracecheck`.
6. Add each new Go file to the `paths` list in `proofs.yml`.
7. Run the axiom audit (Run locally).

## Upgrade Lean and Mathlib

One PR per upgrade, nothing else in it.

1. Pick a Mathlib release tag `vX` (Mathlib tags match Lean releases).
2. Set `rev = "vX"` in `lakefile.toml`; in `_proofs` run `lake update mathlib`.
3. Copy `.lake/packages/mathlib/lean-toolchain` to `_proofs/lean-toolchain`.
4. `lake exe cache get`, then `lake build --wfail`. Fix deprecations and broken proofs; NEVER silence them.
5. Run the axiom audit and `go run ./tracecheck`.
6. Commit `lakefile.toml`, `lean-toolchain`, `lake-manifest.json` and the Lean fixes.

- NEVER run `lake update` without step 2's pinned `rev`; it moves every transitive package.
- Dependabot does not update Lean packages.

## Run locally

Put `~/.elan/bin` on `PATH`. In `_proofs`:

| Step | Command |
|---|---|
| Download Mathlib artifacts | `lake exe cache get` |
| Build, warnings fatal | `lake build --wfail` |
| Imports complete | `lake exe mk_all --check` |
| Trace checker tests | `go vet ./... && go test ./...` |
| Manifest check | `go run ./tracecheck` |

Axiom audit, as lean-action runs it: clone `https://github.com/leanprover-community/axiom-audit` at tag `v0.1.2`
into a scratch directory, check `HEAD` is `46024e005996495c65ef609368e11ab39c4222e3`, copy
`_proofs/lean-toolchain` over its `lean-toolchain`, run `lake build` there, then in `_proofs` run
`lake env <clone>/.lake/build/bin/axiom-audit --allow propext,Classical.choice,Quot.sound --root Sketch`.
