package main

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mtlynch/logpaste/handlers"
	"github.com/mtlynch/logpaste/store/sqlite"
	"golang.org/x/time/rate"
)

const pilotDataDir = "/app/data"

func runPilot() error {
	username := os.Getenv("LP_PILOT_USERNAME")
	password := os.Getenv("LP_PILOT_PASSWORD")
	if username == "" || strings.TrimSpace(username) != username || strings.Contains(username, ":") ||
		len(password) < 24 || strings.TrimSpace(password) != password {
		return errors.New("configure LP_PILOT_USERNAME and a generated LP_PILOT_PASSWORD of at least 24 characters")
	}
	origin, err := url.Parse(os.Getenv("LP_PILOT_PUBLIC_ORIGIN"))
	if err != nil || origin.Hostname() == "" || origin.String() != "https://"+origin.Host ||
		origin.Scheme != "https" || origin.Host != origin.Hostname() || origin.Host != strings.ToLower(origin.Host) {
		return errors.New("set LP_PILOT_PUBLIC_ORIGIN to the canonical HTTPS origin")
	}
	if port := os.Getenv("PORT"); port != "" && port != "3001" {
		return errors.New("the VibeNest pilot requires PORT=3001")
	}
	if os.Getenv("DB_REPLICA_URL") != "" {
		return errors.New("clear DB_REPLICA_URL for the local SQLite pilot")
	}
	if !pilotDataMounted() {
		return errors.New("mount a dedicated writable volume at /app/data")
	}

	entries, err := sqlite.OpenPilot(pilotDataDir)
	if err != nil {
		return err
	}
	defer entries.Close()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if entries.PurgeExpired() != nil {
					log.Print("Pilot SQLite expiry cleanup failed")
				}
			}
		}
	}()

	router := handlers.NewPilot(handlers.SiteProperties{
		Title:    "LogPaste",
		Subtitle: "Share synthetic diagnostic logs with your team.",
	}, 64*1024, entries, origin.String()).Router()
	server := &http.Server{
		Addr:              ":3001",
		Handler:           pilotHandler(router, origin, username, password),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 * 1024,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	log.Print("Starting the VibeNest pilot on port 3001")
	return server.ListenAndServe()
}

func pilotDataMounted() bool {
	info, err := os.Lstat(pilotDataDir)
	if err != nil || !info.IsDir() {
		return false
	}
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	defer mounts.Close()

	found := false
	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		before, after, ok := strings.Cut(scanner.Text(), " - ")
		fields, filesystem := strings.Fields(before), strings.Fields(after)
		if ok && len(fields) >= 6 && len(filesystem) > 0 && fields[4] == pilotDataDir {
			if found || !strings.Contains(","+fields[5]+",", ",rw,") ||
				filesystem[0] == "tmpfs" || filesystem[0] == "ramfs" {
				return false
			}
			found = true
		}
	}
	return found && scanner.Err() == nil
}

func pilotHandler(router http.Handler, origin *url.URL, username, password string) http.Handler {
	wantUser := sha256.Sum256([]byte(username))
	wantPassword := sha256.Sum256([]byte(password))
	active := make(chan struct{}, 4)
	uploads := rate.NewLimiter(rate.Every(10*time.Second), 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/healthz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if !strings.EqualFold(r.Host, origin.Host) {
			http.Error(w, "Unexpected host", http.StatusMisdirectedRequest)
			return
		}
		user, pass, ok := r.BasicAuth()
		gotUser := sha256.Sum256([]byte(user))
		gotPassword := sha256.Sum256([]byte(pass))
		match := subtle.ConstantTimeCompare(wantUser[:], gotUser[:]) &
			subtle.ConstantTimeCompare(wantPassword[:], gotPassword[:])
		if !ok || match != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="LogPaste"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
		default:
			http.Error(w, "Too many active requests", http.StatusTooManyRequests)
			return
		}

		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			if source := r.Header.Get("Origin"); (source != "" && source != origin.String()) ||
				r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Cross-origin uploads are disabled", http.StatusForbidden)
				return
			}
			if !uploads.Allow() {
				w.Header().Set("Retry-After", "10")
				http.Error(w, "Upload limit reached", http.StatusTooManyRequests)
				return
			}
		}
		router.ServeHTTP(w, r)
	})
}
