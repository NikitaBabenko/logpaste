package handlers

import (
	"net/http"

	"github.com/mtlynch/logpaste/limit"
	"github.com/mtlynch/logpaste/store"
	"github.com/mtlynch/logpaste/store/sqlite"
)

type Server interface {
	Router() http.Handler
}

func New(sp SiteProperties, perMinuteLimit int, maxCharLimit int64) Server {
	return newServer(sp, perMinuteLimit, maxCharLimit, sqlite.New("data/store.db"), "")
}

func NewPilot(sp SiteProperties, maxBytes int64, entries store.Store, publicOrigin string) Server {
	sp.PublicOrigin = publicOrigin
	return newServer(sp, limit.RateLimitingDisabled, maxBytes, entries, publicOrigin)
}

func newServer(sp SiteProperties, perMinuteLimit int, maxCharLimit int64, entries store.Store, publicOrigin string) Server {
	s := defaultServer{
		router:        http.NewServeMux(),
		store:         entries,
		publicOrigin:  publicOrigin,
		siteProps:     sp,
		ipRateLimiter: limit.New(perMinuteLimit),
		maxCharLimit:  maxCharLimit,
	}
	s.routes()
	return s
}

type SiteProperties struct {
	Title        string
	Subtitle     string
	FooterHTML   string
	ShowDocs     bool
	PublicOrigin string
}

type defaultServer struct {
	router        *http.ServeMux
	store         store.Store
	siteProps     SiteProperties
	ipRateLimiter limit.IPRateLimiter
	maxCharLimit  int64
	publicOrigin  string
}

// Router returns the underlying router interface for the server.
func (s defaultServer) Router() http.Handler {
	return s.router
}
