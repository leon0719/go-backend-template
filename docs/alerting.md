# Alerting

**Status: mostly not implemented in this version.** This Go template has no Sentry/error-tracking integration and no queue-depth monitoring. It *does* ship a periodic heartbeat task and a scheduler to enqueue it, but that task only writes a log line — nothing external is pinged and nothing alerts. This doc intentionally does not claim otherwise.

If you need this, the natural mapping onto what already exists here is:

- An error-tracking SDK (e.g. Sentry's Go SDK) wired into `internal/logging` — either as a custom `slog.Handler` that also reports `Error`-level records, or invoked directly at error sites.
- A periodic asynq task that pings an external heartbeat URL. **The scheduling half of this now exists**: `cmd/scheduler` runs an `asynq.Scheduler` that enqueues `system:heartbeat` every 5 minutes, and `tasks.HandleHeartbeat` (`internal/tasks/heartbeat.go`) handles it — but it only logs. Make it a real dead-man's switch by GETting a Healthchecks.io/Better Stack URL from that handler.
- A periodic task that reads the asynq queue length (`asynq.Inspector.GetQueueInfo`) and alerts past a threshold.

None of the above is scaffolded here — treat this file as a TODO list, not documentation of existing behavior.
