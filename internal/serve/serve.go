package serve

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/record"
)

const maxNetwork = 200

// Handler serves lookups from an open index and an optional saved run; every route is read-only, so requests run in parallel without locks.
func Handler(ix *index.Index, run *record.Recording) http.Handler {
	mux := http.NewServeMux()
	network := sync.OnceValues(func() (index.Network, error) { return ix.Network(maxNetwork) })
	mux.HandleFunc("GET /lookup", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("domain")
		if strings.TrimSpace(q) == "" {
			writeJSON(w, http.StatusBadRequest, errorBody("missing domain"))
			return
		}
		limit, err := parseLimit(r.URL.Query().Get("limit"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("limit must be a whole number, 0 or more"))
			return
		}
		start := time.Now()
		res, err := ix.Lookup(q, limit)
		if err != nil {
			log.Printf("lookup %q: %v", q, err)
			writeJSON(w, http.StatusInternalServerError, errorBody("index read failed"))
			return
		}
		if res == nil {
			writeJSON(w, http.StatusNotFound, errorBody("no backlinks found"))
			return
		}
		res.TookMs = float64(time.Since(start).Microseconds()) / 1000
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ix.Find(r.URL.Query().Get("domain")); !ok {
			writeJSON(w, http.StatusNotFound, map[string]bool{"known": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"known": true})
	})
	mux.HandleFunc("GET /network", func(w http.ResponseWriter, r *http.Request) {
		n := 60
		if q := r.URL.Query().Get("n"); q != "" {
			v, err := strconv.Atoi(q)
			if err != nil || v < 1 || v > maxNetwork {
				writeJSON(w, http.StatusBadRequest, errorBody("n must be a whole number from 1 to 200"))
				return
			}
			n = v
		}
		nw, err := network()
		if err != nil {
			log.Printf("network: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorBody("index read failed"))
			return
		}
		writeJSON(w, http.StatusOK, nw.Top(n))
	})
	mux.HandleFunc("GET /run", func(w http.ResponseWriter, r *http.Request) {
		if run == nil {
			writeJSON(w, http.StatusNotFound, errorBody("no saved run"))
			return
		}
		writeJSON(w, http.StatusOK, run)
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ix.Manifest())
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	return mux
}

func parseLimit(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}

func errorBody(msg string) map[string]string { return map[string]string{"error": msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
