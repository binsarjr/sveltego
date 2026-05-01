# Golden tests

Golden tests pin codegen and other deterministic output byte-for-byte. They are
the project's first line of defense against silent regressions in the
`.svelte` → `.gen/*.go` pipeline. Spec: [#104](https://github.com/binsarjr/sveltego/issues/104).

## TL;DR

```bash
# 1. Write or change the emitter, then run the suite.
go test ./packages/sveltego/core/codegen
# Fails with a unified diff between expected.go and the new output.

# 2. If the diff is intentional, regenerate goldens.
go test ./packages/sveltego/core/codegen -args -update

# 3. Review every byte that changed.
git diff packages/sveltego/core/codegen/testdata/golden

# 4. Commit only if the diff matches intent.
git add packages/sveltego/core/codegen/testdata/golden
git commit -m "feat(codegen): emit sorted attr keys"
```

The PR reviewer reads the same diff line-by-line. Surprise diffs block merge.

## When to use a golden test

Use a golden test when the output is:

- **Deterministic** — same input always produces byte-identical bytes.
- **Structured text** — Go source, HTML fragments, manifests, JSON.
- **Stable across reviewers** — a human can read the diff and decide if it is
  intentional.

Examples in scope:

- Codegen emitter output (`.svelte` → generated Go).
- AST-to-source transformers (e.g. expression rewriter).
- Manifest serialization (route table, asset graph).

**Out of scope** — these have other test strategies:

- Runtime HTML output of a live SSR request. Covered by integration tests in
  `e2e/` (lands with the perf milestone, see [#60](https://github.com/binsarjr/sveltego/issues/60)).
- Parser AST equality. Covered by table-driven AST tests; the AST is not text.
- Anything stochastic (random IDs, timing, network).

## API

The shared helper lives in
[`github.com/binsarjr/sveltego/packages/sveltego/internal/testutils/golden`](https://github.com/binsarjr/sveltego/tree/main/packages/sveltego/internal/testutils/golden).

```go
import "github.com/binsarjr/sveltego/packages/sveltego/internal/testutils/golden"

func TestCompile(t *testing.T) {
    got, err := codegen.Compile(input)
    if err != nil {
        t.Fatal(err)
    }
    golden.Equal(t, "each-with-index", got)
}
```

`golden.Equal` contract:

- Compares `got` against `testdata/golden/<name>.golden` (flat) or
  `testdata/golden/<name>/expected.go` (scenario layout — see below).
- On mismatch: `t.Errorf` with a unified diff and a hint to run `-update`.
- With the package-level `-update` flag set: rewrites the golden, no diff.
- Calls `t.Helper()` so the failure points at the caller, not the helper.

Read the package godoc for the full surface. Keep helpers thin — golden code
that grows logic is a smell.

## Layout convention

Two layouts. Pick one per package; do not mix.

### Scenario layout (codegen, transformers)

One directory per scenario. `input.<ext>` plus one or more `expected.<ext>`
artifacts:

```
packages/sveltego/core/codegen/
└── testdata/
    └── golden/
        ├── each-simple/
        │   ├── input.svelte
        │   └── expected.go
        ├── each-with-index/
        │   ├── input.svelte
        │   └── expected.go
        └── if-else/
            ├── input.svelte
            └── expected.go
```

Scenario name names the coverage: `each-with-key`, `attr-spread`,
`runes-derived`. The test driver walks `testdata/golden/`, runs each subdir as
a `t.Run` subtest, parallel where safe.

### Flat layout (single-output tools)

One file per case, named after the test:

```
packages/sveltego/cli/manifest/
└── testdata/
    └── golden/
        ├── empty.golden
        ├── single-route.golden
        └── nested-layout.golden
```

Use the flat layout when input is a constant or test-defined struct, not a
file on disk.

`testdata/` always lives **inside the test's own package**. Cross-package
golden directories are forbidden — they break `go test ./pkg/...` isolation
and confuse `-update`.

## Update flow

1. Change the emitter, transformer, or whatever produces the output.
2. Run the suite without `-update` to confirm it fails.

   ```bash
   go test ./packages/sveltego/core/codegen
   ```

3. Regenerate goldens for that package:

   ```bash
   go test ./packages/sveltego/core/codegen -args -update
   ```

   Or rewrite all goldens repo-wide (slower, useful after a cross-cutting
   change like a `gofumpt` bump):

   ```bash
   bash scripts/update-goldens.sh
   ```

4. Inspect the diff:

   ```bash
   git diff packages/sveltego/core/codegen/testdata/golden
   ```

5. Decide:

   - Diff matches intent → `git add` + commit.
   - Diff has anything unexpected → `git checkout -- testdata/` and
     re-investigate. Do not commit a golden you cannot explain.

6. Push. Reviewer reads the same diff and approves byte-by-byte.

## Determinism rules

A golden suite is only as honest as the determinism of its inputs. Four hard
rules per [#104](https://github.com/binsarjr/sveltego/issues/104):

1. **No `time.Now()`** in generated content. Inject a clock or a fixed
   compile-time field.
2. **Sort map keys** before iterating. Range over `maps.Keys()` sorted, never
   raw `for k := range m`.
3. **Use `strconv.FormatFloat`** with explicit precision. `%v` format width
   for floats can shift across Go releases.
4. **No random IDs.** If you need a stable identifier, hash the content
   (FNV or SHA256 of the source).

A separate `TestDeterminism` runs `Compile` against each input N times and
asserts byte-identical output. Treat a flake there as a P0.

## Anti-patterns

- Running `-update` blindly without reading the diff. The whole point of the
  flow is human review at update time.
- Generating non-deterministic output and "fixing" the test by re-running
  until it passes. Fix the source of the non-determinism.
- Placing `testdata/` in a sibling package or shared directory. Goldens move
  with the code that produces them.
- Embedding huge fixtures (>1MB) in `testdata/`. If the input is that large,
  the test is exercising too much surface — split it.
- Skipping the determinism test in the suite. It is cheap insurance.

## References

- Spec: [#104 Codegen golden testing](https://github.com/binsarjr/sveltego/issues/104)
- Helper godoc:
  [`test-utils/golden/doc.go`](https://github.com/binsarjr/sveltego/blob/main/packages/sveltego/internal/testutils/golden/doc.go)
- [Go testing with subtests](https://go.dev/blog/subtests)
- [`gofumpt`](https://github.com/mvdan/gofumpt) — pinned by codegen for
  cross-version stability
