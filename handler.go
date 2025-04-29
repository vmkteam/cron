package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/robfig/cron/v3"
)

type State struct {
	ID            int
	Name          string
	Schedule      string
	IsMaintenance bool
	LastState     string
	LastErr       error
	LastDuration  time.Duration
	LastUpdatedAt time.Time

	LastRun time.Time
	NextRun time.Time
}

type States []State

// LogValue implements slog.LogValuer.
func (s States) LogValue() slog.Value {
	attrs := make([]slog.Attr, len(s))
	for i, state := range s {
		attrs[i] = slog.Group(
			state.Name,
			slog.String("schedule", state.Schedule),
			slog.String("next", state.NextRun.Format(time.RFC3339)),
			slog.String("state", state.LastState),
		)
	}
	return slog.GroupValue(attrs...)
}

// State returns job states.
func (cm *Manager) State() States {
	cm.muState.Lock()
	defer cm.muState.Unlock()

	// get cron entries
	entries := cm.cron.Entries()
	entryIndex := make(map[int]cron.Entry)
	for i := range entries {
		entryIndex[int(entries[i].ID)] = entries[i]
	}

	// get cron jobs
	rr := make([]State, len(cm.jobs))
	for i, job := range cm.jobs {
		s := State{
			ID:            int(job.id),
			Name:          job.name,
			Schedule:      job.schedule.String(),
			IsMaintenance: job.isMaintenance,
			LastState:     string(job.last.state),
			LastErr:       job.last.err,
			LastDuration:  job.last.duration,
			LastUpdatedAt: job.last.updatedAt,
		}

		if e, ok := entryIndex[s.ID]; ok {
			s.LastRun = e.Prev
			s.NextRun = e.Next
		}

		rr[i] = s
	}

	return rr
}

func (cm *Manager) Handler(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		p   printer
	)

	startID := r.URL.Query().Get("start")
	if startID != "" {
		go func() { _ = cm.ManualRun(context.WithoutCancel(r.Context()), startID) }()
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return
	}

	// show info
	state := cm.State()
	acceptHeader := r.Header.Get("Accept")
	switch {
	case strings.Contains(acceptHeader, "application/json"):
		w.Header().Set("Content-Type", "application/json")
		err = p.json(state, w)
	case strings.Contains(acceptHeader, "text/html"):
		w.Header().Set("Content-Type", "text/html")
		err = p.html(state, w)
	default:
		w.Header().Set("Content-Type", "text/plain")
		p.text(state, w)
	}

	p.error(w, err)
}

// TextSchedule writes current cron schedule with TabWriter.
func (cm *Manager) TextSchedule(w io.Writer) {
	printer{}.text(cm.State(), w)
}

// printer is a helper to prints state in json,html or text format.
type printer struct{}

// json writes states as json.
func (printer) json(state []State, w io.Writer) error {
	return json.NewEncoder(w).Encode(state)
}

// error writes 500 http status code and error if not nil.
func (printer) error(w http.ResponseWriter, err error) {
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s\n", err)
	}
}

// text writes states with TabWriter.
func (printer) text(state []State, w io.Writer) {
	wr := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprint(wr, tableRow("cron", "schedule", "next", "state"))
	for _, st := range state {
		next, maintenance := "", ""
		if st.NextRun.IsZero() {
			next = "never"
		} else {
			next = fmt.Sprintf("(starts in %s)", time.Until(st.NextRun))
		}

		if st.IsMaintenance {
			maintenance = " (maintenance)"
		}

		fmt.Fprintf(wr, tableRow("cron=%s%s", "%s", "%s", "%s"), st.Name, maintenance, st.Schedule, next, st.LastState)
	}
	_ = wr.Flush()
}

// tableRow is a helper for tab separated strings.
func tableRow(ss ...string) string {
	for i := range ss {
		ss[i] = "  " + ss[i]
	}
	return strings.Join(ss, "\t") + "\n"
}

// html renders cron UI.
func (printer) html(state []State, w io.Writer) error {
	tmpl, err := template.New("states").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			r := t.Format("2006-01-02 15:04:05")
			d := time.Since(t)
			if d > 0 {
				r += fmt.Sprintf(" (%s ago)", d.Round(time.Second).String())
			} else {
				r += fmt.Sprintf(" (in %s)", d.Round(time.Second).String())
			}
			return r
		},
		"formatDuration": func(d time.Duration) string {
			if d == 0 {
				return ""
			}
			return d.Round(time.Second).String()
		},
		"stateColor": func(state string) string {
			switch state {
			case "running":
				return "background-color: #e6f7ff"
			case "disabled":
				return "background-color: #f5f5f5"
			case "skipped":
				return "background-color: #fff7e6"
			case "idle":
				return "background-color: #e6ffed"
			default:
				return ""
			}
		},
		"formatName": func(name string, isMaintenance bool) string {
			if isMaintenance {
				return name + " (maintenance)"
			}
			return name
		},
		"formatNextRun": func(nextRun time.Time) string {
			if nextRun.IsZero() {
				return ""
			}
			duration := time.Until(nextRun)
			if duration < 0 {
				return "overdue"
			}
			return nextRun.Format("2006-01-02 15:04:05") +
				" (in " + duration.Round(time.Second).String() + ")"
		},
		"isOverdue": func(nextRun time.Time) bool {
			return !nextRun.IsZero() && nextRun.Before(time.Now())
		},
	}).Parse(htmlTemplate)
	if err != nil {
		return err
	}

	return tmpl.Execute(w, state)
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>Cron Tasks Status</title>
    <meta http-equiv="refresh" content="10">
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            color: #333;
        }
        table {
            border-collapse: collapse;
            width: 100%;
            margin-top: 20px;
        }
        th, td {
            border: 1px solid #ddd;
            padding: 8px 12px;
            text-align: left;
        }
        th {
            background-color: #f8f9fa;
            font-weight: 600;
        }
        td.center {
			text-align: center;
        }
        td.right {
			text-align: right;
        }
        tr:hover {
            background-color: #f5f5f5;
        }
        .action-link {
            color: #1a73e8;
            text-decoration: none;
        }
        .action-link:hover {
            text-decoration: underline;
        }
        .overdue {
            color: #d32f2f;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <h1>Cron Tasks Status</h1>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Schedule</th>
                <th>State</th>
                <th>Last Error</th>
                <th>Duration</th>
                <th>Updated</th>
                <th>Last Run</th>
                <th>Next Run</th>
                <th>Action</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}
            <tr style="{{.LastState | stateColor}}">
                <td>{{.ID}}</td>
                <td>{{ formatName .Name .IsMaintenance}}</td>
                <td class="center">{{.Schedule}}</td>
                <td class="center">{{.LastState}}</td>
                <td>{{if .LastErr}}{{.LastErr.Error}}{{end}}</td>
                <td class="right">{{.LastDuration | formatDuration}}</td>
                <td>{{.LastUpdatedAt | formatTime}}</td>
                <td>{{.LastRun | formatTime}}</td>
                <td {{if isOverdue .NextRun}}class="overdue"{{end}}>
                    {{formatNextRun .NextRun}}
                </td>
                <td><a href="?start={{.Name}}" class="action-link">Run</a></td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>
</html>`
