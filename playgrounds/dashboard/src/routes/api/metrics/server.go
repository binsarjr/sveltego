//go:build sveltego

package metrics

import (
	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

// GET returns the latest synthetic time-series the polling chart
// renders. The series is process-scoped (in-memory ring buffer in
// store.Default) so repeated requests show motion.
func GET(ev *kit.RequestEvent) *kit.Response {
	_ = ev
	samples := store.Default.Metrics()
	ts := make([]string, len(samples))
	values := make([]int, len(samples))
	for i, s := range samples {
		ts[i] = s.TS.UTC().Format("2006-01-02T15:04:05Z")
		values[i] = s.Value
	}
	return kit.JSON(200, kit.M{
		"ts":     ts,
		"values": values,
	})
}
