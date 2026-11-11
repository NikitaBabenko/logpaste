package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/mtlynch/logpaste/random"
	"github.com/mtlynch/logpaste/store"
)

type PastePutResponse struct {
	Id string `json:"id"`
}

func (s defaultServer) pasteGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		contents, err := s.store.GetEntry(id)
		if err != nil {
			if _, ok := errors.AsType[store.EntryNotFoundError](err); ok {
				http.Error(w, "entry not found", http.StatusNotFound)
				return
			}
			s.logf("failed to retrieve entry %s from datastore: %v", id, err)
			http.Error(w, "failed to retrieve entry", http.StatusInternalServerError)
			return
		}
		if _, err := io.WriteString(w, contents); err != nil {
			s.logf("failed to write response: %v", err)
		}
	}
}

func (s defaultServer) pasteOptions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
}

func (s defaultServer) pastePut() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.publicOrigin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		bodyRaw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxCharLimit))
		if err != nil {
			s.logf("Error reading body: %v", err)
			s.bodyError(w, err, "can't read request body")
			return
		}

		body := string(bodyRaw)
		if !s.validatePaste(body, w) {
			return
		}

		id := generateEntryId()
		if !s.savePaste(w, id, body) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := PastePutResponse{
			Id: id,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
	}
}

func (s defaultServer) pastePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const multipartOverhead = 1024
		r.Body = http.MaxBytesReader(w, r.Body, s.maxCharLimit+multipartOverhead)
		maxMemory := s.maxCharLimit
		if s.publicOrigin != "" {
			// The whole pilot request fits in memory, including any uploaded file.
			maxMemory += multipartOverhead
		}
		if err := r.ParseMultipartForm(maxMemory); err != nil {
			s.logf("failed to parse form: %v", err)
			s.bodyError(w, err, "no valid multipart/form-data found")
			return
		}
		if s.publicOrigin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		body, ok := s.parsePasteFromMultipartForm(r.MultipartForm)
		if !ok {
			s.logf("form did not contain any recognizable data")
			http.Error(w, "form data or file is required", http.StatusBadRequest)
			return
		}

		if !s.validatePaste(body, w) {
			return
		}

		id := generateEntryId()
		if !s.savePaste(w, id, body) {
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		origin := s.publicOrigin
		if origin == "" {
			origin = baseURLFromRequest(r)
		}
		resultURL := fmt.Sprintf("%s/%s\n", origin, id)
		if _, err := w.Write([]byte(resultURL)); err != nil {
			s.logf("failed to write response: %v", err)
		}
	}
}

func (s defaultServer) logf(format string, args ...any) {
	if s.publicOrigin == "" {
		log.Printf(format, args...)
	}
}

func (s defaultServer) bodyError(w http.ResponseWriter, err error, message string) {
	if _, oversized := errors.AsType[*http.MaxBytesError](err); s.publicOrigin != "" && oversized {
		http.Error(w, "Paste is too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, message, http.StatusBadRequest)
}

func (s defaultServer) savePaste(w http.ResponseWriter, id, body string) bool {
	err := s.store.InsertEntry(id, body)
	if errors.Is(err, store.ErrCapacity) {
		http.Error(w, "Paste storage is full", http.StatusInsufficientStorage)
		return false
	}
	if err != nil {
		s.logf("failed to save entry: %v", err)
		http.Error(w, "can't save entry", http.StatusInternalServerError)
		return false
	}
	s.logf("saved entry of %d characters", len(body))
	return true
}

func generateEntryId() string {
	return random.String(8)
}

func (s defaultServer) validatePaste(p string, w http.ResponseWriter) bool {
	if len(strings.TrimSpace(p)) == 0 {
		s.logf("Paste body was empty")
		http.Error(w, "empty body", http.StatusBadRequest)
		return false
	} else if int64(len(p)) > s.maxCharLimit {
		s.logf("Paste body was too long: %d characters", len(p))
		s.bodyError(w, &http.MaxBytesError{Limit: s.maxCharLimit}, "body too long")
		return false
	}
	return true
}

func (s defaultServer) parsePasteFromMultipartForm(f *multipart.Form) (string, bool) {
	if content, ok := s.anyValueInForm(f); ok {
		return content, true
	}
	if content, ok := s.parsePasteFromMultipartFormFile(f); ok {
		return content, true
	}
	return "", false
}

func (s defaultServer) anyValueInForm(f *multipart.Form) (string, bool) {
	for _, values := range f.Value {
		if len(values) < 1 {
			s.logf("form values are empty")
			continue
		}
		return values[0], true
	}
	return "", false
}

func (s defaultServer) parsePasteFromMultipartFormFile(f *multipart.Form) (string, bool) {
	file, ok := s.anyFileInForm(f.File)
	if !ok {
		return "", false
	}

	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		s.logf("failed to read form file: %v", err)
		return "", false
	}

	return string(body), true
}

func (s defaultServer) anyFileInForm(formFiles map[string][]*multipart.FileHeader) (multipart.File, bool) {
	for _, fileHeaders := range formFiles {
		if len(fileHeaders) < 1 {
			s.logf("form files are empty")
			continue
		}
		file, err := fileHeaders[0].Open()
		if err != nil {
			s.logf("failed to open form file: %v", err)
			return nil, false
		}
		return file, true
	}
	return nil, false
}

func baseURLFromRequest(r *http.Request) string {
	var scheme string
	// If we're running behind a proxy, assume that it's a TLS proxy.
	if r.TLS != nil || os.Getenv("LP_BEHIND_PROXY") != "" {
		scheme = "https"
	} else {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
