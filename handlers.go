package main

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

type app struct {
	db    *sql.DB
	sched *Scheduler
}

func (a *app) auth(h http.HandlerFunc) http.HandlerFunc {
	return requireAuth(a.db, h)
}

// ---- login / logout ----

func (a *app) loginGet(w http.ResponseWriter, r *http.Request) {
	if validSession(a.db, r) {
		http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
		return
	}
	render(w, "login.html", map[string]any{"ShowNav": false})
}

func (a *app) loginPost(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if !verifyTOTP(code) {
		render(w, "login.html", map[string]any{"ShowNav": false, "Error": "Invalid code."})
		return
	}
	if err := createSession(a.db, w); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (a *app) logoutPost(w http.ResponseWriter, r *http.Request) {
	destroySession(a.db, w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ---- shared helpers ----

func lastRun(db *sql.DB, sourceType string, id int64) (status, at string) {
	var exitCode int
	var started time.Time
	err := db.QueryRow(`SELECT exit_code, started_at FROM runs WHERE source_type = ? AND source_id = ? ORDER BY started_at DESC LIMIT 1`,
		sourceType, id).Scan(&exitCode, &started)
	if err != nil {
		return "", ""
	}
	if exitCode == 0 {
		status = "ok"
	} else {
		status = "failed"
	}
	return status, started.Format("2006-01-02 15:04:05")
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// ---- webhook routes ----

type webhookRow struct {
	ID                 int64
	Path, Name         string
	LastStatus, LastAt string
}

func (a *app) webhooksList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, path, name FROM webhook_routes ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var routes []webhookRow
	for rows.Next() {
		var wr webhookRow
		if err := rows.Scan(&wr.ID, &wr.Path, &wr.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		routes = append(routes, wr)
	}
	rows.Close()

	// lastRun issues its own query, so it must run after the rows above are
	// fully drained — SetMaxOpenConns(1) means an open Rows starves any query
	// issued while it's still being iterated.
	for i := range routes {
		routes[i].LastStatus, routes[i].LastAt = lastRun(a.db, "webhook", routes[i].ID)
	}
	render(w, "webhooks_list.html", map[string]any{"ShowNav": true, "Routes": routes})
}

type webhookRecord struct {
	ID                        int64
	Path, Name, Script, Token string
}

func (a *app) webhooksNewForm(w http.ResponseWriter, r *http.Request) {
	render(w, "webhooks_form.html", map[string]any{"ShowNav": true, "Action": "/webhooks", "Route": webhookRecord{}})
}

func (a *app) webhooksCreate(w http.ResponseWriter, r *http.Request) {
	path, name, script := r.FormValue("path"), r.FormValue("name"), r.FormValue("script")
	if path == "" || name == "" || script == "" {
		render(w, "webhooks_form.html", map[string]any{"ShowNav": true, "Action": "/webhooks", "Error": "All fields are required.", "Route": webhookRecord{Path: path, Name: name, Script: script}})
		return
	}
	token, err := randomHex(32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := a.db.Exec(`INSERT INTO webhook_routes (path, name, script, token) VALUES (?, ?, ?, ?)`, path, name, script, token); err != nil {
		render(w, "webhooks_form.html", map[string]any{"ShowNav": true, "Action": "/webhooks", "Error": "Could not save (path must be unique): " + err.Error(), "Route": webhookRecord{Path: path, Name: name, Script: script}})
		return
	}
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (a *app) getWebhook(id int64) (webhookRecord, error) {
	var wr webhookRecord
	err := a.db.QueryRow(`SELECT id, path, name, script, token FROM webhook_routes WHERE id = ?`, id).
		Scan(&wr.ID, &wr.Path, &wr.Name, &wr.Script, &wr.Token)
	return wr, err
}

func (a *app) webhooksEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	wr, err := a.getWebhook(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, "webhooks_form.html", map[string]any{"ShowNav": true, "Edit": true, "Action": "/webhooks/" + r.PathValue("id"), "Route": wr})
}

func (a *app) webhooksUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name, script := r.FormValue("name"), r.FormValue("script")
	if name == "" || script == "" {
		wr, _ := a.getWebhook(id)
		wr.Name, wr.Script = name, script
		render(w, "webhooks_form.html", map[string]any{"ShowNav": true, "Edit": true, "Action": "/webhooks/" + r.PathValue("id"), "Error": "Name and script are required.", "Route": wr})
		return
	}
	if _, err := a.db.Exec(`UPDATE webhook_routes SET name = ?, script = ? WHERE id = ?`, name, script, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (a *app) webhooksDelete(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.db.Exec(`DELETE FROM webhook_routes WHERE id = ?`, id)
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (a *app) webhooksRun(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	wr, err := a.getWebhook(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	runScript(a.db, "webhook", wr.ID, wr.Script)
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

// ---- cron jobs ----

type cronRow struct {
	ID                 int64
	Name, Schedule     string
	Enabled            bool
	NextRun            string
	LastStatus, LastAt string
}

func (a *app) cronList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, name, schedule, enabled FROM cron_jobs ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []cronRow
	for rows.Next() {
		var cr cronRow
		if err := rows.Scan(&cr.ID, &cr.Name, &cr.Schedule, &cr.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if next, ok := a.sched.nextRun(cr.ID); ok {
			cr.NextRun = next.Format("2006-01-02 15:04:05")
		}
		jobs = append(jobs, cr)
	}
	rows.Close()

	// see webhooksList: lastRun's query must run after rows is drained,
	// not while SetMaxOpenConns(1) has it checked out.
	for i := range jobs {
		jobs[i].LastStatus, jobs[i].LastAt = lastRun(a.db, "cron", jobs[i].ID)
	}
	render(w, "cron_list.html", map[string]any{"ShowNav": true, "Jobs": jobs})
}

type cronRecord struct {
	ID                     int64
	Name, Schedule, Script string
	Enabled                bool
}

func (a *app) cronNewForm(w http.ResponseWriter, r *http.Request) {
	render(w, "cron_form.html", map[string]any{"ShowNav": true, "Action": "/cron", "Job": cronRecord{Enabled: true}})
}

func (a *app) cronCreate(w http.ResponseWriter, r *http.Request) {
	name, schedule, script := r.FormValue("name"), r.FormValue("schedule"), r.FormValue("script")
	enabled := r.FormValue("enabled") == "on"
	if name == "" || schedule == "" || script == "" {
		render(w, "cron_form.html", map[string]any{"ShowNav": true, "Action": "/cron", "Error": "All fields are required.", "Job": cronRecord{Name: name, Schedule: schedule, Script: script, Enabled: enabled}})
		return
	}
	res, err := a.db.Exec(`INSERT INTO cron_jobs (name, schedule, script, enabled) VALUES (?, ?, ?, ?)`, name, schedule, script, enabled)
	if err != nil {
		render(w, "cron_form.html", map[string]any{"ShowNav": true, "Action": "/cron", "Error": "Could not save: " + err.Error(), "Job": cronRecord{Name: name, Schedule: schedule, Script: script, Enabled: enabled}})
		return
	}
	id, _ := res.LastInsertId()
	if enabled {
		if err := a.sched.add(id, schedule, script); err != nil {
			a.db.Exec(`UPDATE cron_jobs SET enabled = 0 WHERE id = ?`, id)
			render(w, "cron_form.html", map[string]any{"ShowNav": true, "Action": "/cron", "Error": "Invalid cron expression: " + err.Error(), "Job": cronRecord{Name: name, Schedule: schedule, Script: script, Enabled: false}})
			return
		}
	}
	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

func (a *app) getCronJob(id int64) (cronRecord, error) {
	var cr cronRecord
	err := a.db.QueryRow(`SELECT id, name, schedule, script, enabled FROM cron_jobs WHERE id = ?`, id).
		Scan(&cr.ID, &cr.Name, &cr.Schedule, &cr.Script, &cr.Enabled)
	return cr, err
}

func (a *app) cronEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cr, err := a.getCronJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, "cron_form.html", map[string]any{"ShowNav": true, "Edit": true, "Action": "/cron/" + r.PathValue("id"), "Job": cr})
}

func (a *app) cronUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name, schedule, script := r.FormValue("name"), r.FormValue("schedule"), r.FormValue("script")
	enabled := r.FormValue("enabled") == "on"
	if name == "" || schedule == "" || script == "" {
		cr, _ := a.getCronJob(id)
		cr.Name, cr.Schedule, cr.Script, cr.Enabled = name, schedule, script, enabled
		render(w, "cron_form.html", map[string]any{"ShowNav": true, "Edit": true, "Action": "/cron/" + r.PathValue("id"), "Error": "All fields are required.", "Job": cr})
		return
	}
	if enabled {
		if err := a.sched.reload(id, schedule, script, true); err != nil {
			cr, _ := a.getCronJob(id)
			cr.Name, cr.Schedule, cr.Script, cr.Enabled = name, schedule, script, enabled
			render(w, "cron_form.html", map[string]any{"ShowNav": true, "Edit": true, "Action": "/cron/" + r.PathValue("id"), "Error": "Invalid cron expression: " + err.Error(), "Job": cr})
			return
		}
	} else {
		a.sched.remove(id)
	}
	if _, err := a.db.Exec(`UPDATE cron_jobs SET name = ?, schedule = ?, script = ?, enabled = ? WHERE id = ?`, name, schedule, script, enabled, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

func (a *app) cronDelete(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.sched.remove(id)
	a.db.Exec(`DELETE FROM cron_jobs WHERE id = ?`, id)
	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

func (a *app) cronToggle(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cr, err := a.getCronJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newEnabled := !cr.Enabled
	if err := a.sched.reload(id, cr.Schedule, cr.Script, newEnabled); err != nil {
		http.Redirect(w, r, "/cron", http.StatusSeeOther)
		return
	}
	a.db.Exec(`UPDATE cron_jobs SET enabled = ? WHERE id = ?`, newEnabled, id)
	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

func (a *app) cronRun(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cr, err := a.getCronJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	runScript(a.db, "cron", cr.ID, cr.Script)
	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

// ---- run history ----

type runRow struct {
	StartedAt              string
	SourceType, SourceName string
	SourceID               int64
	ExitCode               int
	Stdout, Stderr         string
}

func (a *app) runsList(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")

	query := `SELECT source_type, source_id, started_at, exit_code, stdout, stderr FROM runs`
	args := []any{}
	if source == "webhook" || source == "cron" {
		query += ` WHERE source_type = ?`
		args = append(args, source)
	}
	query += ` ORDER BY started_at DESC LIMIT 200`

	rows, err := a.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var runs []runRow
	for rows.Next() {
		var rr runRow
		var started time.Time
		if err := rows.Scan(&rr.SourceType, &rr.SourceID, &started, &rr.ExitCode, &rr.Stdout, &rr.Stderr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rr.StartedAt = started.Format("2006-01-02 15:04:05")
		runs = append(runs, rr)
	}
	rows.Close()

	// see webhooksList: sourceName's query must run after rows is drained,
	// not while SetMaxOpenConns(1) has it checked out.
	for i := range runs {
		runs[i].SourceName = a.sourceName(runs[i].SourceType, runs[i].SourceID)
	}
	render(w, "runs.html", map[string]any{"ShowNav": true, "Runs": runs})
}

func (a *app) sourceName(sourceType string, id int64) string {
	table := "cron_jobs"
	if sourceType == "webhook" {
		table = "webhook_routes"
	}
	var name string
	a.db.QueryRow(`SELECT name FROM `+table+` WHERE id = ?`, id).Scan(&name)
	return name
}

// ---- public webhook trigger ----

func (a *app) hookTrigger(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	var id int64
	var script, token string
	err := a.db.QueryRow(`SELECT id, script, token FROM webhook_routes WHERE path = ?`, path).Scan(&id, &script, &token)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	given := r.Header.Get("X-Deploy-Token")
	if subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	exitCode, _, _ := runScript(a.db, "webhook", id, script)
	w.WriteHeader(http.StatusOK)
	if exitCode == 0 {
		w.Write([]byte("ok\n"))
	} else {
		w.Write([]byte("script exited with code " + strconv.Itoa(exitCode) + "\n"))
	}
}
