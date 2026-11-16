package trace

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestLog(t *testing.T) {
	var out bytes.Buffer
	l := New(&out)
	l.Say("check", "example.com/a", "broken (%d)", 404)
	l.Add(Event{Step: "decide", Text: "picked /b", Thinking: "line one\nline two"})
	ev := l.Events()
	if len(ev) != 2 || ev[0].Text != "broken (404)" || ev[0].Page != "example.com/a" || ev[1].Thinking == "" {
		t.Fatalf("events = %+v", ev)
	}
	for _, want := range []string{"check     example.com/a: broken (404)", "decide    picked /b", "thinking:", "  line two"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestNilLog(t *testing.T) {
	var l *Log
	l.Say("x", "", "ignored")
	if l.Events() != nil {
		t.Error("nil log returned events")
	}
}

func TestLogConcurrent(t *testing.T) {
	l := New(nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { l.Say("check", "", "ok") })
	}
	wg.Wait()
	if n := len(l.Events()); n != 20 {
		t.Errorf("%d events, want 20", n)
	}
}
