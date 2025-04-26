package main

import (
	"context"
	"fmt"

	"log"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vmkteam/cron"
)

// newTask creates new random task with random sleep (0-70s) and with random error or panic.
func newTask(msg string) cron.Func {
	return func(ctx context.Context) error {
		sec := rand.IntN(70)
		log.Printf("%v: started (%vsec)\n", msg, sec)
		time.Sleep(time.Duration(sec) * time.Second)
		log.Printf("%v: finished\n", msg)

		if rand.IntN(10) == 5 {
			return fmt.Errorf("%v: job finished with random error", msg)
		}

		if rand.IntN(10) == 2 {
			panic("job test panic")
		}

		return nil
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stderr)

	sl := NewLogger(false)
	ctx := context.Background()
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

	// add simple func
	m.AddFunc("f1", "* * * * *", newTask("f1"))
	m.AddFunc("f2", "* * * * *", newTask("f2"))
	m.AddFunc("f5", "", newTask("f5"))
	m.AddMaintenanceFunc("f3", "*/2 * * * *", newTask("f3m"))

	// run cron
	if err := m.Run(ctx); err != nil {
		log.Fatal(err)
	}

	// print schedule (two variants)
	m.TextSchedule(os.Stdout)
	sl.Print(ctx, "cron initialized", "job", m.State())
	sl.Print(ctx, "open this url for cron ui", "url", "http://localhost:2112/debug/cron")
	sl.Print(ctx, "open this url for metrics", "url", "http://localhost:2112/metrics")

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/debug/cron", m.Handler)
	err := http.ListenAndServe(":2112", nil)
	log.Println(err)
}

// Logger is a simple text/json Slog Logger.
type Logger struct {
	*slog.Logger
}

func NewLogger(json bool) Logger {
	l := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if json {
		l = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	return Logger{
		Logger: l,
	}
}

func (l Logger) Print(ctx context.Context, msg string, args ...any) {
	l.InfoContext(ctx, msg, args...)
}
func (l Logger) Error(ctx context.Context, msg string, args ...any) {
	l.ErrorContext(ctx, msg, args...)
}
