# Alerting

**Status: not implemented in this version.** This Go template does not have Sentry/error-tracking integration, a heartbeat check, or queue-depth monitoring — this doc intentionally does not claim otherwise.

If you need this, the natural mapping onto what already exists here is:

- An error-tracking SDK (e.g. Sentry's Go SDK) wired into `internal/logging` — either as a custom `slog.Handler` that also reports `Error`-level records, or invoked directly at error sites.
- A periodic asynq task that pings an external heartbeat URL.
- A periodic task that reads the asynq queue length (`asynq.Inspector.GetQueueInfo`) and alerts past a threshold.

None of the above is scaffolded here — treat this file as a TODO list, not documentation of existing behavior.
