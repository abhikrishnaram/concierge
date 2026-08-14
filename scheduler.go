package main

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler wraps robfig/cron so cron_jobs CRUD from the admin UI can update
// the live schedule immediately, not just on next restart.
type Scheduler struct {
	db      *sql.DB
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[int64]cron.EntryID
}

func newScheduler(db *sql.DB) *Scheduler {
	c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	return &Scheduler{db: db, cron: c, entries: map[int64]cron.EntryID{}}
}

func (s *Scheduler) Start() { s.cron.Start() }

// loadAll registers every enabled cron job. Call once at startup.
func (s *Scheduler) loadAll() error {
	rows, err := s.db.Query(`SELECT id, schedule, script FROM cron_jobs WHERE enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var schedule, script string
		if err := rows.Scan(&id, &schedule, &script); err != nil {
			return err
		}
		if err := s.add(id, schedule, script); err != nil {
			log.Printf("cron: failed to schedule job %d: %v", id, err)
		}
	}
	return rows.Err()
}

func (s *Scheduler) add(id int64, schedule, script string) error {
	entryID, err := s.cron.AddFunc(schedule, func() {
		runScript(s.db, "cron", id, script)
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[id] = entryID
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) remove(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, id)
	}
}

// reload re-registers job id, replacing any existing entry. Call after
// add/edit/toggle so the running scheduler reflects the change immediately.
func (s *Scheduler) reload(id int64, schedule, script string, enabled bool) error {
	s.remove(id)
	if !enabled {
		return nil
	}
	return s.add(id, schedule, script)
}

func (s *Scheduler) nextRun(id int64) (time.Time, bool) {
	s.mu.Lock()
	entryID, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return time.Time{}, false
	}
	entry := s.cron.Entry(entryID)
	if entry.ID == 0 {
		return time.Time{}, false
	}
	return entry.Next, true
}
