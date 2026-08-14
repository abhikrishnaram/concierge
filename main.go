package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "totp-reset" {
		runTOTPReset()
		return
	}

	mustParsePages()

	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sched := newScheduler(db)
	if err := sched.loadAll(); err != nil {
		log.Fatal(err)
	}
	sched.Start()

	a := &app{db: db, sched: sched}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /hooks/{path}", a.hookTrigger)

	mux.HandleFunc("GET /login", a.loginGet)
	mux.HandleFunc("POST /login", a.loginPost)
	mux.HandleFunc("POST /logout", a.logoutPost)

	mux.HandleFunc("GET /{$}", a.auth(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
	}))

	mux.HandleFunc("GET /webhooks", a.auth(a.webhooksList))
	mux.HandleFunc("GET /webhooks/new", a.auth(a.webhooksNewForm))
	mux.HandleFunc("POST /webhooks", a.auth(a.webhooksCreate))
	mux.HandleFunc("GET /webhooks/{id}/edit", a.auth(a.webhooksEditForm))
	mux.HandleFunc("POST /webhooks/{id}", a.auth(a.webhooksUpdate))
	mux.HandleFunc("POST /webhooks/{id}/delete", a.auth(a.webhooksDelete))
	mux.HandleFunc("POST /webhooks/{id}/run", a.auth(a.webhooksRun))

	mux.HandleFunc("GET /cron", a.auth(a.cronList))
	mux.HandleFunc("GET /cron/new", a.auth(a.cronNewForm))
	mux.HandleFunc("POST /cron", a.auth(a.cronCreate))
	mux.HandleFunc("GET /cron/{id}/edit", a.auth(a.cronEditForm))
	mux.HandleFunc("POST /cron/{id}", a.auth(a.cronUpdate))
	mux.HandleFunc("POST /cron/{id}/delete", a.auth(a.cronDelete))
	mux.HandleFunc("POST /cron/{id}/toggle", a.auth(a.cronToggle))
	mux.HandleFunc("POST /cron/{id}/run", a.auth(a.cronRun))

	mux.HandleFunc("GET /runs", a.auth(a.runsList))

	addr := ":" + port()
	log.Println("concierge listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
