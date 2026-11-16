package trace

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Event is one step the agent took, in the order it happened.
type Event struct {
	At       float64 `json:"at"`
	Step     string  `json:"step"`
	Page     string  `json:"page,omitempty"`
	Text     string  `json:"text"`
	Thinking string  `json:"thinking,omitempty"`
}

// Log collects events and, if Out is set, prints each one as it happens. A nil *Log ignores everything.
type Log struct {
	Out    io.Writer
	start  time.Time
	mu     sync.Mutex
	events []Event
}

func New(out io.Writer) *Log { return &Log{Out: out, start: time.Now()} }

// Say records one step; page may be empty.
func (l *Log) Say(step, page, format string, args ...any) {
	l.Add(Event{Step: step, Page: page, Text: fmt.Sprintf(format, args...)})
}

// Add records an event, stamping it with the seconds since the log started.
func (l *Log) Add(e Event) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e.At = float64(time.Since(l.start).Milliseconds()) / 1000
	l.events = append(l.events, e)
	if l.Out != nil {
		print(l.Out, e)
	}
}

// Events returns a copy of everything recorded so far.
func (l *Log) Events() []Event {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Event(nil), l.events...)
}

func print(w io.Writer, e Event) {
	text := e.Text
	if e.Page != "" {
		text = e.Page + ": " + text
	}
	fmt.Fprintf(w, "%7.1fs  %-9s %s\n", e.At, e.Step, text)
	if e.Thinking == "" {
		return
	}
	fmt.Fprintf(w, "%7s  %-9s thinking:\n", "", "")
	for _, line := range strings.Split(strings.TrimSpace(e.Thinking), "\n") {
		fmt.Fprintf(w, "%7s  %-9s   %s\n", "", "", line)
	}
}
