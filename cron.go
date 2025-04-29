package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	maintenanceKey contextKey = "maintenance"
	nameKey        contextKey = "name"

	stateIdle     cronState = "idle"
	stateDisabled cronState = "disabled"
	stateRunning  cronState = "running"
	stateSkipped  cronState = "skipped"
)

var (
	ErrSkipped   = errors.New("skipped")
	ErrNotFound  = errors.New("job not found")
	ErrDuplicate = errors.New("duplicate cron name")
)

type (
	contextKey string
	cronState  string

	Func           func(ctx context.Context) error
	MiddlewareFunc func(Func) Func
	LogPrintf      func(format string, v ...interface{})

	Runner interface {
		Run(context.Context) error
	}
)

type Schedule string

func (ss Schedule) String() string { return string(ss) }
func (ss Schedule) IsActive() bool { return ss != Schedule(stateDisabled) && ss != "" }

// Manager is a Cron manager with context and middleware support.
type Manager struct {
	cron       *cron.Cron
	middleware []MiddlewareFunc
	jobs       []job
	muState    sync.Mutex
}

type job struct {
	id            cron.EntryID // cron id after AddFunc in robfig/cron
	name          string
	schedule      Schedule
	isMaintenance bool
	fn            Func
	cronFn        Func

	// last states
	last jobState
}

type jobState struct {
	state     cronState
	err       error
	updatedAt time.Time
	duration  time.Duration
}

func NewManager() *Manager {
	return &Manager{
		cron: cron.New(),
	}
}

// AddFunc adds func to cron.
func (cm *Manager) AddFunc(name string, schedule Schedule, fn Func) {
	cm.jobs = append(cm.jobs, newJob(name, schedule, fn, false))
}

// Add adds Runner to cron.
func (cm *Manager) Add(name string, schedule Schedule, r Runner) {
	cm.AddFunc(name, schedule, r.Run)
}

// AddMaintenanceFunc adds func to cron.
func (cm *Manager) AddMaintenanceFunc(name string, schedule Schedule, fn Func) {
	cm.jobs = append(cm.jobs, newJob(name, schedule, fn, true))
}

// validateJobs checks jobs for unique names.
func (cm *Manager) validateJobs() (string, error) {
	names := make(map[string]struct{}, len(cm.jobs))
	for _, job := range cm.jobs {
		// check for duplicates
		n := strings.ToLower(job.name)
		if _, ok := names[n]; ok {
			return job.name, ErrDuplicate
		}
		names[n] = struct{}{}

		// parse schedule
		if job.schedule.IsActive() {
			_, err := cron.ParseStandard(job.schedule.String())
			if err != nil {
				return job.name, err
			}
		}
	}
	return "", nil
}

// ManualRun runs a cron func with middlewares and context.
func (cm *Manager) ManualRun(ctx context.Context, id string) error {
	for i := range cm.jobs {
		if strings.EqualFold(cm.jobs[i].name, id) {
			// run found func
			return cm.jobs[i].cronFn(ctx)
		}
	}

	return ErrNotFound
}

// Run is a main function that registers all jobs and starts robfig/cron in separate goroutine.
func (cm *Manager) Run(ctx context.Context) error {
	// check for duplicate names and schedule error.
	if name, err := cm.validateJobs(); name != "" {
		return fmt.Errorf("%w: %s", err, name)
	}

	// register functions
	for idx := range cm.jobs {
		j := cm.jobs[idx]

		// create main job function
		cronFnCtx := func(ctx context.Context) error {
			// set middleware to func
			f := j.fn
			for i := len(cm.middleware) - 1; i >= 0; i-- {
				f = cm.middleware[i](f)
			}

			// set context
			ctx = NewNameContext(ctx, j.name)
			ctx = NewMaintenanceContext(ctx, j.isMaintenance)

			// invoke main func with middleware
			cm.updateState(idx, stateRunning, nil)
			err := f(ctx)
			cm.updateState(idx, stateIdle, err)

			return err
		}
		// check for disabled schedule. save cronFn to job for manual run
		if !j.schedule.IsActive() {
			cm.updateID(idx, cron.EntryID(idx*-1), cronFnCtx) // set fake id
			cm.updateState(idx, stateDisabled, nil)
			continue
		}

		// register main functions in cron library
		id, err := cm.cron.AddFunc(j.schedule.String(), func() { _ = cronFnCtx(ctx) })
		if err != nil {
			return fmt.Errorf("add cron=%v failed: %w", j.name, err)
		}

		// set ID
		cm.updateID(idx, id, cronFnCtx)
	}

	// run main cron process in its own go routine
	cm.cron.Start()

	return nil
}

// Stop stops current cron instance.
func (cm *Manager) Stop() context.Context {
	if cm.cron == nil {
		return context.Background()
	}

	return cm.cron.Stop()
}

// updateState set.
func (cm *Manager) updateState(idx int, state cronState, err error) {
	cm.muState.Lock()
	defer cm.muState.Unlock()

	last := cm.jobs[idx].last

	// set dur when state changed from running to idle.
	if last.state == stateRunning && state == stateIdle {
		last.duration = time.Since(last.updatedAt)
	}

	// do not set idle state if skipped
	last.state, last.err = state, err
	last.updatedAt = time.Now()

	// check for Skipped Err
	if errors.Is(err, ErrSkipped) {
		last.state, last.err = stateSkipped, nil
	}

	// fix state
	cm.jobs[idx].last = last
}

// updateID sets cron.EntryID for job.
func (cm *Manager) updateID(idx int, id cron.EntryID, funcJob Func) {
	cm.muState.Lock()
	defer cm.muState.Unlock()

	cm.jobs[idx].id = id
	cm.jobs[idx].cronFn = funcJob
}

// Use adds middleware for cron job.
func (cm *Manager) Use(m ...MiddlewareFunc) {
	cm.middleware = append(cm.middleware, m...)
}

// newJob returns new job.
func newJob(name string, schedule Schedule, fn Func, isMaintenance bool) job {
	return job{
		name:          name,
		schedule:      schedule,
		fn:            fn,
		isMaintenance: isMaintenance,
		last: jobState{
			state: stateIdle,
		},
	}
}

func NewMaintenanceContext(ctx context.Context, isMaintenance bool) context.Context {
	return context.WithValue(ctx, maintenanceKey, isMaintenance)
}

func MaintenanceFromContext(ctx context.Context) bool {
	if r, ok := ctx.Value(maintenanceKey).(bool); ok {
		return r
	}

	return false
}

func NewNameContext(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, nameKey, name)
}

func NameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(nameKey).(string); ok {
		return v
	}

	return ""
}
