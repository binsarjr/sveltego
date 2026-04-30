# bench baselines

Captured `go test -bench=.` output checked in for CI regression comparison.

`baseline.txt` is the reference run; CI's `bench.yml` invokes
`benchstat baseline.txt new.txt` and fails the job when any benchmark
regresses past the configured threshold.

## Refresh procedure

Update baselines when an intentional perf change lands on `main` and the
old numbers are no longer representative:

```sh
go test -bench=. -benchmem -count=10 -run='^$' ./bench/. | tee bench/baseline/baseline.txt
```

Commit the regenerated file in the same PR that introduced the change.
Reviewers verify the diff matches the announced delta.

## Hardware drift

Numbers are recorded on the maintainer's box (Apple M1 Pro, darwin/arm64)
and on the CI runner. The CI gate compares HEAD against HEAD~1 from the
same runner — host drift is bounded to the runner image. The checked-in
baseline exists for human reference and local sanity checks; CI does not
diff against it directly.
