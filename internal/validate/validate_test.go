package validate

import (
	"math"
	"strings"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/pagerank"
)

func TestReverseDomain(t *testing.T) {
	for in, want := range map[string]string{"com.facebook": "facebook.com", "uk.co.bbc": "bbc.co.uk", "io.github.alice": "alice.github.io", "localhost": "localhost"} {
		if got := reverseDomain(in); got != want {
			t.Errorf("reverseDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadRanks(t *testing.T) {
	in := "#harmonicc_pos\t#harmonicc_val\t#pr_pos\t#pr_val\t#host_rev\t#n_hosts\n" +
		"1\t3.0E7\t3\t0.0149\tcom.googleapis\t7733\n" +
		"2\t2.9E7\t1\t0.0200\tuk.co.bbc\t50\n"
	ref, err := ReadRanks(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if r := ref["bbc.co.uk"]; r.PRPos != 1 || r.PR != 0.02 {
		t.Fatalf("bbc.co.uk = %+v", r)
	}
	if len(ref) != 2 {
		t.Fatalf("got %d domains", len(ref))
	}
	if _, err := ReadRanks(strings.NewReader("1\t2\tx\t0.1\tcom.a\t1\n")); err == nil {
		t.Fatal("expected an error for a bad pr_pos")
	}
	if _, err := ReadRanks(strings.NewReader("")); err == nil {
		t.Fatal("expected an error for an empty file")
	}
}

func TestSpearman(t *testing.T) {
	cases := []struct {
		name string
		x, y []float64
		want float64
	}{
		{"same order", []float64{1, 2, 3, 4}, []float64{10, 20, 30, 40}, 1},
		{"reversed", []float64{1, 2, 3, 4}, []float64{4, 3, 2, 1}, -1},
		{"only order matters", []float64{1, 2, 3, 4}, []float64{1, 100, 1000, 1e9}, 1},
		{"one swap", []float64{1, 2, 3, 4, 5}, []float64{1, 2, 3, 5, 4}, 0.9},
	}
	for _, c := range cases {
		if got := Spearman(c.x, c.y); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %f, want %f", c.name, got, c.want)
		}
	}
	if !math.IsNaN(Spearman([]float64{1}, []float64{1})) {
		t.Error("one point should give NaN")
	}
}

func TestRanksAverageTies(t *testing.T) {
	got := ranks([]float64{5, 1, 5, 3})
	want := []float64{3.5, 1, 3.5, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	ours := []pagerank.Score{
		{Domain: "a.com", Rank: 0.5, LinkingDomains: 10},
		{Domain: "b.com", Rank: 0.3, LinkingDomains: 5},
		{Domain: "c.com", Rank: 0.1, LinkingDomains: 1},
		{Domain: "notincc.com", Rank: 0.05, LinkingDomains: 1},
	}
	ref := map[string]Reference{"a.com": {PRPos: 1, PR: 0.9}, "b.com": {PRPos: 2, PR: 0.5}, "c.com": {PRPos: 3, PR: 0.1}}
	rep := Compare(ours, ref, []int{1, 5}, 3)

	if lv := rep.Levels[0]; lv.Domains != 4 || lv.Matched != 3 || math.Abs(lv.Spearman-1) > 1e-9 {
		t.Fatalf("level 1 = %+v", lv)
	}
	if lv := rep.Levels[1]; lv.Domains != 2 || lv.Matched != 2 {
		t.Fatalf("level 5 = %+v", lv)
	}
	if len(rep.Top) != 3 || rep.Top[0].Domain != "a.com" || rep.Top[0].CCPos != 1 || rep.Top[2].OurPos != 3 {
		t.Fatalf("top = %+v", rep.Top)
	}
}
