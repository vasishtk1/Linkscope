# internal/record

Saves a finished pipeline run to one JSON file, so the website can replay it with a Play button long after the run is over, and on a hosted site where no pipeline is running.

```
coordinator's jobs.db ──▶ cmd/record ──▶ data/run.json ──▶ cmd/api GET /run ──▶ website /pipeline (Play)
```

**Why a file and not a live connection:** a run happens once, on whatever machine runs the pipeline. Saving it makes it a few KB that ships with the API, so anyone visiting the site can watch what happened. Every number in it comes from the run itself.

## record.go

### `Recording`

| Field | Meaning |
|---|---|
| `Name` | Label shown on the website, e.g. "20 Common Crawl files, 4 workers" |
| `RecordedAt` | When it was saved |
| `Derived` | `true` when the database had no event log, so the history was rebuilt from each finished file's timing (see `derive`) |
| `Files` | Every file: id, path, final state, attempts used, pages, links and site pairs |
| `Workers` | Every worker that appears in the events, sorted |
| `Events` | The coordinator's event log (`jobs.Event`), oldest first. The replay is computed entirely from this |
| `Totals` | Files, done, failed, pages, links, site pairs, failed attempts (failures plus expired leases) and the run's length in seconds, first event to last |

A 20-file run is about 5–10 KB.

### `File`, `Totals`

The rows and totals above.

### `FromStore(s *jobs.Store, name string, now time.Time) (Recording, error)`

Reads every job (`store.List`) and the event log (`store.Events(0)`), and adds up the totals. Pages, links and site pairs only count finished files, matching the coordinator's `/status`.

### `derive(rows []jobs.JobRow) []jobs.Event` (unexported)

For databases made before the event log existed. Each finished file becomes two events: claimed at `finished_at − duration_ms` and completed at `finished_at`, on the worker that held it. Sorted by time and numbered. Failed attempts and retries from those runs are lost, which is why the recording is marked `Derived` and `cmd/record` says so.

## Tests

| Test | Checks |
|---|---|
| `TestFromStore` | A run with a retry that worked and a file that failed twice: totals, failed attempts, run length, workers, file states |
| `TestDeriveFromFinishedJobs` | Unfinished files skipped, claim times worked out from durations, events sorted by time and numbered, details kept |
