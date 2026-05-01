package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// Log keys used exclusively by cron task logging.
const (
	logKeyName = "name"
	logKeySpec = "spec"
)

// runCronTasks starts one goroutine per task. Each goroutine fires the task
// on its parsed interval and exits when ctx is cancelled. Errors from the
// task function are logged via logger but do not stop the loop — a failing
// task keeps retrying on its normal schedule. Tasks whose Spec fails to
// parse are logged and skipped rather than aborting server startup.
func runCronTasks(ctx context.Context, tasks []kit.CronTask, logger *slog.Logger) {
	for _, task := range tasks {
		interval, err := kit.ParseSchedule(task.Spec)
		if err != nil {
			logger.Error("server: cron task skipped — bad spec",
				logKeyError, err.Error(),
				logKeyName, task.Name,
				logKeySpec, task.Spec,
			)
			continue
		}
		go runCronLoop(ctx, task, interval, logger)
	}
}

// runCronLoop ticks at interval and calls task.Fn until ctx is done.
func runCronLoop(ctx context.Context, task kit.CronTask, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	name := task.Name
	if name == "" {
		name = task.Spec
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := task.Fn(ctx); err != nil {
				logger.Error("server: cron task error",
					logKeyError, err.Error(),
					logKeyName, name,
				)
			}
		}
	}
}
