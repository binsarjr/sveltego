# bench baselines

Captured `go test -bench=.` output checked in for CI regression comparison.

`baseline.txt` is the all-modes reference run. The four `baseline-<mode>.txt`
files (`ssr`, `ssg`, `spa`, `static`) carry the per-mode subsets so the
regression gate (#105) can compare each mode independently — preventing an
SSR perf gain from masking an SSG regression and vice versa.

CI's `bench.yml` invokes `benchstat <base> <new>` and fails the job when
any benchmark regresses past the configured threshold.

## Refresh procedure

Update baselines when an intentional perf change lands on `main` and the
old numbers are no longer representative:

```sh
# All-modes refresh (default reference for human review).
go test -bench=. -benchmem -count=6 -run='^$' ./bench/. \
  | tee bench/baseline/baseline.txt

# Per-mode refresh — used by the regression gate.
go test -bench='^BenchmarkServeHTTP_(Hello|List|Detail|Action|SvelteSPA|SSRHello|SSRTypical|SSRHeavy)$' \
  -benchmem -count=6 -run='^$' ./bench/. \
  | tee bench/baseline/baseline-ssr.txt

go test -bench='^BenchmarkServeHTTP_SSGServe$' \
  -benchmem -count=6 -run='^$' ./bench/. \
  | tee bench/baseline/baseline-ssg.txt

go test -bench='^BenchmarkServeHTTP_SPAShell$' \
  -benchmem -count=6 -run='^$' ./bench/. \
  | tee bench/baseline/baseline-spa.txt

go test -bench='^BenchmarkServeHTTP_StaticNoLoad$' \
  -benchmem -count=6 -run='^$' ./bench/. \
  | tee bench/baseline/baseline-static.txt
```

Commit the regenerated files in the same PR that introduced the change.
Reviewers verify the diff matches the announced delta.

## Hardware drift

Numbers are recorded on the maintainer's box (Apple M1 Pro, darwin/arm64)
and on the CI runner. The CI gate compares HEAD against HEAD~1 from the
same runner — host drift is bounded to the runner image. The checked-in
baseline exists for human reference and local sanity checks; CI does not
diff against it directly.
