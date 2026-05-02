# 2026-05-03 — TestStreaming_CancelPropagatesWithinDeadline flake (#435)

## Insight

The test asserted that a `kit.StreamCtx` producer goroutine exits within
1 second after the request context is cancelled. The cancel was
delivered indirectly: the test cancelled the **client** request context,
relied on `net/http`'s client transport to close the TCP connection,
relied on `net/http`'s server-side connection state poller to detect the
EOF on its background reader goroutine, then waited for the resulting
server `r.Context()` cancel to propagate through the kit context tree
to the producer goroutine.

Under heavy parallel test load (CI shared runners; locally reproducible
with `GOMAXPROCS=1 -parallel=64 -count=200 ./server/`), the slow link in
that chain is `net/http`'s background reader goroutine. It competes for
CPU with dozens of other tests' goroutines and may take well over a
second to be scheduled, observe the EOF, and fire the per-request
cancel. The producer is blameless — the parent context never ticks
inside the deadline.

The "1s deadline" was therefore measuring `net/http` connection-teardown
latency under contention, not the cancel-propagation property the test
name claims. Hypothesis in #435 ("bump to 2-3s or use synctest") would
have masked the misdirection without fixing it: any deadline is still
gated by net/http scheduling under enough load.

## Fix

Decouple the test from the HTTP transport. The Load handler now derives
a cancellable child of `lctx.Request.Context()`, sends the cancel func
to the test over a channel, and uses that derived context as the
`StreamCtx` parent. The test cancels via the captured func — direct
context cancel, no transport hop. The deadline still applies (2s, with
margin for goroutine scheduling jitter); a producer that ignores its
context will still hang and fail the test.

The HTTP request still runs in the background to drive Load through the
real pipeline, but its disconnect timing is no longer load-bearing.

Verified: `go test -race -count=100` passes 100/100 on the focused test.
Full server-package run (`-count=300` default GOMAXPROCS) shows zero
failures of this test.

## Self-rules

1. When asserting a property of code we own (here: parent-context cancel
   propagating to a child goroutine), construct the test so the
   stimulus reaches the code-under-test directly. Going through a
   stdlib transport (`net/http` client → server disconnect detection)
   adds steps whose latency is bounded only by the host scheduler
   and OS. Those steps are not what the test claims to measure.

2. Time-bounded "did the goroutine exit?" assertions in `-parallel=N`
   integration tests need a budget that absorbs scheduler jitter for
   the goroutines they actually depend on. 1s is fine for a direct
   context cancel; 1s is **not** fine when the cancel path crosses
   net/http connection teardown under CI contention.

3. Bumping a flaky test's deadline without naming the slow path in the
   chain is a band-aid. The slow path may not be in the code-under-test
   at all — bumping hides that misattribution and the test keeps
   "asserting" something it doesn't actually exercise.

4. When a flake reproduces only under `-parallel=N` with high N, the
   bug is rarely in the test's nominal subject. It is usually in some
   off-path goroutine the test silently depends on (background readers,
   accept loops, finalizers). Trace the *actual* wakeup chain before
   adjusting timeouts.
