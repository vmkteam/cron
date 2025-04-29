# cron manager

[![Linter Status](https://github.com/vmkteam/cron/actions/workflows/golangci-lint.yml/badge.svg?branch=master)](https://github.com/vmkteam/cron/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/vmkteam/cron)](https://goreportcard.com/report/github.com/vmkteam/cron)
[![Go Reference](https://pkg.go.dev/badge/github.com/vmkteam/cron.svg)](https://pkg.go.dev/github.com/vmkteam/cron)

A robust cron job manager built on [robfig/cron](https://github.com/robfig/cron) with enhanced features for production use.

## Key Features
* **Context-aware jobs:** func(ctx context.Context) error signature
* **Built-in UI:** Web interface for monitoring and control
* **Manual execution:** API endpoints for on-demand job runs
* **Middleware support**: Extensible pipeline for job processing
* **Enhanced state tracking:** Detailed job status monitoring
* **Schedule visualization:** Tools for schedule inspection



## Middlewares
* `WithLogger` Traditional logging via Printf function.
* `WithSLog` Logs job execution via slog.
* `WithSentry` Reports errors to Sentry (includes panic recovery).
* `WithRecover` Recovers from panics (alternative to Sentry).
* `WithDevel` Marks development environment in context.
* `WithSkipActive` Prevents parallel execution of the same job.
* `WithMaintenance` Ensures exclusive execution for maintenance jobs.
* `WithMetrics` Tracks execution metrics (count, duration, active jobs).

## Built-in UI Preview
![Web UI](/examples/webui.png)

### curl support

Run `curl http://localhost:2112/debug/cron` for schedule.
```
cron                   |  schedule     |  next                    |  state
cron=f1                |  * * * * *    |  (starts in 16.505033s)  |  idle
cron=f2                |  * * * * *    |  (starts in 16.505028s)  |  idle
cron=f5                |               |  never                   |  disabled
cron=f3 (maintenance)  |  */2 * * * *  |  (starts in 16.505025s)  |  idle
```

Run `curl -L http://localhost:2112/debug/cron?start=<name>` for manual job run.

Run `curl -H 'Accept: application/json' http://localhost:2112/debug/cron` for json output.

## `WithMetrics` Middleware 

* `app_cron_evaluated_total` – total processed jobs by state.
* `app_cron_active` – active running jobs.
* `app_cron_evaluated_duration_seconds` – summary metric with durations.

## Example

Please see `examples/main.go` for basic usage.

```go
    m := cron.NewManager()
    m.Use(
        cron.WithMetrics("test"),
        cron.WithDevel(false),
        cron.WithSLog(sl),
        cron.WithLogger(log.Printf, "test-run"),
        cron.WithMaintenance(log.Printf),
        cron.WithSkipActive(),
        cron.WithRecover(), // recover() inside
        cron.WithSentry(),  // recover() inside
    )
    
    // add simple funcs
    m.AddFunc("f1", "* * * * *", newTask("f1"))
    m.AddFunc("f2", "* * * * *", newTask("f2"))
    m.AddFunc("f5", "", newTask("f5"))
    m.AddMaintenanceFunc("f3", "*/2 * * * *", newTask("f3m"))
    
    // run cron
    if err := m.Run(ctx); err != nil {
        log.Fatal(err)
    }
    
    http.HandleFunc("/debug/cron", m.Handler)
```
