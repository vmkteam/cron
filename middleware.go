package cron

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	isDevelCtx contextKey = "isDevelKey"
)

// WithLogger logs via Printf function (e.g. log.Printf) all runs.
func WithLogger(pf LogPrintf, managerName string) MiddlewareFunc {
	return func(next Func) Func {
		return func(ctx context.Context) error {
			start := time.Now()
			err := next(ctx)

			// set error msg for %q
			errMsg, state := "", "finished"
			if errors.Is(err, ErrSkipped) {
				state = "skipped"
			} else if err != nil {
				errMsg = err.Error()
			}

			pf("cron job %s job=%s duration=%v err=%q manager=%s maintenance=%v",
				state,
				NameFromContext(ctx),
				time.Since(start),
				errMsg,
				managerName,
				MaintenanceFromContext(ctx),
			)
			return err
		}
	}
}

// Logger is as simple interface for slog.
type Logger interface {
	Print(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

// WithSLog logs all runs via slog (see Logger interface).
func WithSLog(lg Logger) MiddlewareFunc {
	return func(next Func) Func {
		return func(ctx context.Context) error {
			start := time.Now()
			err := next(ctx)

			d, name := time.Since(start), NameFromContext(ctx)
			switch {
			case errors.Is(err, ErrSkipped):
				lg.Print(ctx, "cron job skipped", "job", name, "duration", d)
			case err != nil:
				lg.Error(ctx, "cron job failed", "job", name, "duration", d, "err", err)
			default:
				lg.Print(ctx, "cron job finished", "job", name, "duration", d)
			}

			return err
		}
	}
}

// WithSentry sends all errors to sentry. It's also handles panics.
func WithSentry() MiddlewareFunc {
	return func(next Func) Func {
		return func(ctx context.Context) (err error) {
			defer func() {
				var rec any
				if rec = recover(); rec != nil {
					switch e := rec.(type) {
					case error:
						err = e
					default:
						err = fmt.Errorf("%v", e)
					}
				}

				if err != nil {
					sentryHub := sentry.CurrentHub().Clone()
					sentryHub.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("cron", NameFromContext(ctx))
					})
					sentryHub.CaptureException(err)
				}
			}()

			return next(ctx)
		}
	}
}

// WithRecover use recover() func. Do not use with WithSentry middleware due to recover() call.
func WithRecover() MiddlewareFunc {
	return func(next Func) Func {
		return func(ctx context.Context) (err error) {
			// recover
			defer func() {
				if rec := recover(); rec != nil {
					stack := make([]byte, 64<<10)
					stack = stack[:runtime.Stack(stack, false)]
					err = fmt.Errorf("panic: %v: %s", rec, stack)
				}
			}()

			return next(ctx)
		}
	}
}

// WithDevel sets bool flag to context for detecting development environment.
func WithDevel(isDevel bool) MiddlewareFunc {
	return func(h Func) Func {
		return func(ctx context.Context) error {
			ctx = NewIsDevelContext(ctx, isDevel)
			return h(ctx)
		}
	}
}

// NewIsDevelContext creates new context with isDevel flag.
func NewIsDevelContext(ctx context.Context, isDevel bool) context.Context {
	return context.WithValue(ctx, isDevelCtx, isDevel)
}

// IsDevelFromContext returns isDevel flag from context.
func IsDevelFromContext(ctx context.Context) bool {
	if isDevel, ok := ctx.Value(isDevelCtx).(bool); ok {
		return isDevel
	}
	return false
}

// WithSkipActive skips funcs if they are already running.
func WithSkipActive() MiddlewareFunc {
	active := map[string]struct{}{}
	mu := sync.Mutex{}

	return func(next Func) Func {
		return func(ctx context.Context) error {
			name := NameFromContext(ctx)

			// check for running function
			mu.Lock()
			if _, ok := active[name]; ok {
				mu.Unlock()
				return ErrSkipped
			}

			// set active name
			active[name] = struct{}{}
			mu.Unlock()
			defer func() {
				mu.Lock()
				delete(active, name)
				mu.Unlock()
			}()

			// run func
			return next(ctx)
		}
	}
}

// WithMaintenance puts cron jobs in line, got exclusive lock for maintenance job.
func WithMaintenance(p LogPrintf) MiddlewareFunc {
	mutex := sync.RWMutex{}
	pf := func(format string, v ...interface{}) {
		if p != nil {
			p(format, v...)
		}
	}

	return func(next Func) Func {
		return func(ctx context.Context) error {
			name, isMaintenance := NameFromContext(ctx), MaintenanceFromContext(ctx)
			if isMaintenance {
				pf("cron getting maintenance lock=%v", name)
				mutex.Lock()
				pf("cron got maintenance lock=%v", name)
			} else {
				mutex.RLock()
			}

			err := next(ctx)
			if isMaintenance {
				mutex.Unlock()
			} else {
				mutex.RUnlock()
			}

			return err
		}
	}
}

// WithMetrics tracks total/active/duration metrics for runs.
func WithMetrics(app string) MiddlewareFunc {
	statEvaluated := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "app",
		Subsystem: "cron",
		Name:      "evaluated_total",
		Help:      "Track all evaluations of cron.",
	}, []string{"app", "cron", "state"})

	statActive := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "app",
		Subsystem: "cron",
		Name:      "active",
		Help:      "Track current status of cron.",
	}, []string{"app", "cron"})

	statDurations := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "app",
		Subsystem: "cron",
		Name:      "evaluated_duration_seconds",
		Help:      "Response time by cron.",
	}, []string{"app", "cron", "state"})

	prometheus.MustRegister(statEvaluated, statActive, statDurations)

	return func(next Func) Func {
		return func(ctx context.Context) error {
			name, start, state := NameFromContext(ctx), time.Now(), "ok"

			statActive.WithLabelValues(app, name).Inc()
			err := next(ctx)
			if err != nil {
				state = "error"
			}

			statActive.WithLabelValues(app, name).Dec()
			statEvaluated.WithLabelValues(app, name, state).Inc()
			statDurations.WithLabelValues(app, name, state).Observe(time.Since(start).Seconds())

			return err
		}
	}
}
