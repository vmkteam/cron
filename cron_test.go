package cron

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func newCronFunc(msg string) Func {
	return func(ctx context.Context) error {
		fmt.Printf("msg: %v", msg)
		return nil
	}
}

func TestManager_Validate(t *testing.T) {
	Convey("Test validate function", t, func() {
		m := NewManager()

		// add simple func
		m.AddFunc("f1", "0 0 * * *", newCronFunc("f1"))
		m.AddFunc("f2", "0 0 * * *", newCronFunc("f2"))

		Convey("Test duplicate name", func() {
			name, err := m.validateJobs()
			So(err, ShouldBeNil)
			So(name, ShouldBeEmpty)

			m.AddFunc("f2", "0 0 * * *", newCronFunc("f3"))
			n2, err := m.validateJobs()

			So(err, ShouldNotBeNil)
			So(n2, ShouldEqual, "f2")
		})

		Convey("Test invalid schedule", func() {
			m.AddFunc("f3", "invalid", newCronFunc("f3"))
			n2, err := m.validateJobs()

			So(err, ShouldNotBeNil)
			So(n2, ShouldEqual, "f3")
		})
	})
}

func TestManager_Options(t *testing.T) {
	Convey("Test NewManager with custom parser", t, func() {
		m := NewManager(WithParseOptions(Second | Minute | Hour | Dom | Month | Dow | Descriptor))

		Convey("When standard schedule format", func() {
			m.AddFunc("f1", "0 0 * * *", newCronFunc("f1"))

			name, err := m.validateJobs()
			So(err, ShouldNotBeNil)
			So(name, ShouldEqual, "f1")
		})

		Convey("When custom schedule format", func() {
			m.AddFunc("f2", "10 * * * * *", newCronFunc("f1"))

			_, err := m.validateJobs()
			So(err, ShouldBeNil)
		})
	})

	Convey("Test NewManager with standard parser", t, func() {
		m := NewManager()

		Convey("When standard schedule format", func() {
			m.AddFunc("f1", "0 0 * * *", newCronFunc("f1"))

			_, err := m.validateJobs()
			So(err, ShouldBeNil)
		})

		Convey("When custom schedule format", func() {
			m.AddFunc("f2", "10 * * * * *", newCronFunc("f1"))

			name, err := m.validateJobs()
			So(err, ShouldNotBeNil)
			So(name, ShouldEqual, "f2")
		})
	})
}

func TestManager_Run(t *testing.T) {
	Convey("Test validate function", t, func() {
		ctx := t.Context()
		m := NewManager()
		m.Use(
			WithDevel(false),
			WithLogger(log.Printf, "test-run"),
			WithMetrics("test"),
			WithSkipActive(),
			WithMaintenance(log.Printf),
		)

		// add simple func
		m.AddFunc("f1", "0 0 * * *", newCronFunc("f1"))
		m.AddFunc("f2", "0 0 * * *", newCronFunc("f2"))

		Convey("Test run", func() {
			err := m.Run(ctx)
			So(err, ShouldBeNil)
			time.Sleep(1 * time.Second)
			m.Stop()
		})
	})
}
