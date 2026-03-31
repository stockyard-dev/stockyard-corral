package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stockyard-dev/stockyard-corral/internal/store"
)

type Server struct {
	db       *store.DB
	mux      *http.ServeMux
	port     int
	subs     sync.Map // endpoint_id → []*subscriber
	client   *http.Client
	limits   Limits
}

type subscriber struct {
	ch     chan []byte
	cancel func()
}

func New(db *store.DB, port int, limits Limits) *Server {
	s := &Server{
		db:     db,
		mux:    http.NewServeMux(),
		port:   port,
		client: &http.Client{Timeout: 30 * time.Second},
		limits: limits,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Endpoints
	s.mux.HandleFunc("GET /api/endpoints", s.handleListEndpoints)
	s.mux.HandleFunc("POST /api/endpoints", s.handleCreateEndpoint)
	s.mux.HandleFunc("GET /api/endpoints/{id}", s.handleGetEndpoint)
	s.mux.HandleFunc("DELETE /api/endpoints/{id}", s.handleDeleteEndpoint)

	// Events
	s.mux.HandleFunc("GET /api/endpoints/{id}/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/events/{id}", s.handleGetEvent)

	// Replay
	s.mux.HandleFunc("POST /api/events/{id}/replay", s.handleReplayEvent)

	// Forward rules
	s.mux.HandleFunc("GET /api/endpoints/{id}/forwards", s.handleListForwards)
	s.mux.HandleFunc("POST /api/endpoints/{id}/forwards", s.handleCreateForward)

	// Live stream (SSE)
	s.mux.HandleFunc("GET /api/endpoints/{id}/stream", s.handleStream)

	// Webhook receiver (the actual ingest URL)
	s.mux.HandleFunc("/hook/{id}", s.handleWebhookIngest)
	s.mux.HandleFunc("/hook/{id}/{path...}", s.handleWebhookIngest)

	// Status
	s.mux.HandleFunc("GET /api/endpoints/{id}/export", s.handleExportEvents)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Version
	s.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"product": "stockyard-corral", "version": "0.1.0"})
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[corral] listening on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

// --- Endpoint handlers ---

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	eps, err := s.db.ListEndpoints()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if eps == nil {
		eps = []store.Endpoint{}
	}
	writeJSON(w, 200, map[string]any{"endpoints": eps, "count": len(eps)})
}

func (s *Server) handleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Desc string `json:"description"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "endpoint-" + time.Now().Format("20060102-150405")
	}
	if s.limits.MaxEndpoints > 0 {
		eps, _ := s.db.ListEndpoints()
		if LimitReached(s.limits.MaxEndpoints, len(eps)) {
			writeJSON(w, 402, map[string]string{"error": "free tier limit: " + itoa(s.limits.MaxEndpoints) + " endpoints max — upgrade to Pro", "upgrade": "https://stockyard.dev/corral/"})
			return
		}
	}
	ep, err := s.db.CreateEndpoint(req.Name, req.Desc)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	ep.Events = 0
	hookURL := fmt.Sprintf("http://localhost:%d/hook/%s", s.port, ep.ID)
	writeJSON(w, 201, map[string]any{"endpoint": ep, "hook_url": hookURL})
}

func (s *Server) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, err := s.db.GetEndpoint(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "endpoint not found"})
		return
	}
	hookURL := fmt.Sprintf("http://localhost:%d/hook/%s", s.port, ep.ID)
	writeJSON(w, 200, map[string]any{"endpoint": ep, "hook_url": hookURL})
}

func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	s.db.DeleteEndpoint(r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Event handlers ---

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if q := r.URL.Query().Get("q"); q != "" && !s.limits.EventSearch {
		writeJSON(w, 402, map[string]string{"error": "event search requires Pro — upgrade at https://stockyard.dev/corral/", "upgrade": "https://stockyard.dev/corral/"})
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	events, err := s.db.ListEvents(r.PathValue("id"), limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if events == nil {
		events = []store.Event{}
	}
	writeJSON(w, 200, map[string]any{"events": events, "count": len(events)})
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	evt, err := s.db.GetEvent(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}
	// Parse headers for display
	var headers any
	json.Unmarshal([]byte(evt.Headers), &headers)
	writeJSON(w, 200, map[string]any{
		"event": evt, "headers_parsed": headers,
	})
}

// --- Replay ---

func (s *Server) handleReplayEvent(w http.ResponseWriter, r *http.Request) {
	if !s.limits.ReplayHistory {
		writeJSON(w, 402, map[string]string{"error": "replay history requires Pro — upgrade at https://stockyard.dev/corral/", "upgrade": "https://stockyard.dev/corral/"})
		return
	}
	evt, err := s.db.GetEvent(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Target == "" {
		writeJSON(w, 400, map[string]string{"error": "target URL required"})
		return
	}

	start := time.Now()
	httpReq, _ := http.NewRequest(evt.Method, req.Target, strings.NewReader(evt.Body))
	if evt.CType != "" {
		httpReq.Header.Set("Content-Type", evt.CType)
	}
	httpReq.Header.Set("X-Corral-Replay", evt.ID)

	resp, err := s.client.Do(httpReq)
	latency := int(time.Since(start).Milliseconds())
	var statusCode int
	var respBody, errMsg string
	if err != nil {
		errMsg = err.Error()
	} else {
		statusCode = resp.StatusCode
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		respBody = string(body)
	}

	rplID, _ := s.db.RecordReplay(evt.ID, req.Target, statusCode, respBody, latency, errMsg)
	writeJSON(w, 200, map[string]any{
		"replay_id": rplID, "status_code": statusCode, "latency_ms": latency,
		"error": errMsg, "response_size": len(respBody),
	})
}

// --- Forward rules ---

func (s *Server) handleListForwards(w http.ResponseWriter, r *http.Request) {
	rules, _ := s.db.ListForwardRules(r.PathValue("id"))
	if rules == nil {
		rules = []store.ForwardRule{}
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (s *Server) handleCreateForward(w http.ResponseWriter, r *http.Request) {
	epID := r.PathValue("id")
	var req struct {
		Target  string `json:"target_url"`
		FiltHdr string `json:"filter_header"`
		FiltVal string `json:"filter_value"`
		Retries int    `json:"retry_count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Target == "" {
		writeJSON(w, 400, map[string]string{"error": "target_url required"})
		return
	}
	if s.limits.MaxForwardTargets > 0 {
		existing, _ := s.db.ListForwardRules(epID)
		if LimitReached(s.limits.MaxForwardTargets, len(existing)) {
			writeJSON(w, 402, map[string]string{"error": "free tier limit: " + itoa(s.limits.MaxForwardTargets) + " forward target per endpoint — upgrade to Pro", "upgrade": "https://stockyard.dev/corral/"})
			return
		}
	}
	if !s.limits.RetryDeliveries {
		req.Retries = 0 // free tier: no auto-retry
	} else if req.Retries <= 0 {
		req.Retries = 3
	}
	s.db.CreateForwardRule(epID, req.Target, req.FiltHdr, req.FiltVal, req.Retries)
	writeJSON(w, 201, map[string]string{"status": "created"})
}

// --- Webhook ingest ---

func (s *Server) handleWebhookIngest(w http.ResponseWriter, r *http.Request) {
	epID := r.PathValue("id")
	subPath := r.PathValue("path")

	// Verify endpoint exists
	_, err := s.db.GetEndpoint(epID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "endpoint not found"})
		return
	}

	// Read body (limit 1MB)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "failed to read body"})
		return
	}

	// Capture headers
	hdrs := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			hdrs[k] = v[0]
		}
	}
	headersJSON, _ := json.Marshal(hdrs)

	path := "/" + subPath
	sourceIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		sourceIP = strings.Split(fwd, ",")[0]
	}

	evt, err := s.db.RecordEvent(epID, r.Method, path, string(headersJSON), string(body), r.Header.Get("Content-Type"), sourceIP)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to record event"})
		return
	}

	log.Printf("[corral] %s %s %s (%d bytes) from %s", evt.ID, r.Method, epID, len(body), sourceIP)

	// Notify SSE subscribers
	s.notifySubscribers(epID, evt)

	// Execute forward rules asynchronously
	go s.executeForwards(epID, evt)

	writeJSON(w, 200, map[string]any{"status": "received", "event_id": evt.ID})
}

func (s *Server) executeForwards(epID string, evt *store.Event) {
	rules, err := s.db.ListForwardRules(epID)
	if err != nil || len(rules) == 0 {
		return
	}

	for _, rule := range rules {
		// Check filter
		if rule.FiltHdr != "" {
			var hdrs map[string]string
			json.Unmarshal([]byte(evt.Headers), &hdrs)
			val := hdrs[rule.FiltHdr]
			if rule.FiltVal != "" && !strings.Contains(val, rule.FiltVal) {
				continue // Filter doesn't match
			}
		}

		maxAttempts := rule.Retries
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
		backoff := 1 * time.Second

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				time.Sleep(backoff)
				backoff *= 2
			}

			start := time.Now()
			req, _ := http.NewRequest(evt.Method, rule.Target, bytes.NewReader([]byte(evt.Body)))
			if evt.CType != "" {
				req.Header.Set("Content-Type", evt.CType)
			}
			req.Header.Set("X-Corral-Event", evt.ID)
			req.Header.Set("X-Corral-Endpoint", epID)
			req.Header.Set("X-Corral-Attempt", strconv.Itoa(attempt))

			resp, err := s.client.Do(req)
			latency := int(time.Since(start).Milliseconds())
			var status int
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			} else {
				status = resp.StatusCode
				resp.Body.Close()
			}

			s.db.LogForward(rule.ID, evt.ID, status, latency, attempt, errMsg)

			if err == nil && status >= 200 && status < 300 {
				break // Success
			}
		}
	}
}

// --- SSE live stream ---

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	epID := r.PathValue("id")
	ch := make(chan []byte, 64)
	sub := &subscriber{ch: ch}

	// Add subscriber
	val, _ := s.subs.LoadOrStore(epID, &[]*subscriber{})
	subs := val.(*[]*subscriber)
	*subs = append(*subs, sub)

	defer func() {
		// Remove subscriber
		val, ok := s.subs.Load(epID)
		if ok {
			subs := val.(*[]*subscriber)
			for i, ss := range *subs {
				if ss == sub {
					*subs = append((*subs)[:i], (*subs)[i+1:]...)
					break
				}
			}
		}
		close(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "event: connected\ndata: {\"endpoint\":\"%s\"}\n\n", epID)
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: webhook\ndata: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) notifySubscribers(epID string, evt *store.Event) {
	val, ok := s.subs.Load(epID)
	if !ok {
		return
	}
	subs := val.(*[]*subscriber)
	data, _ := json.Marshal(map[string]any{
		"event_id": evt.ID, "method": evt.Method, "path": evt.Path,
		"body_size": evt.BodySize, "content_type": evt.CType,
		"source_ip": evt.SourceIP, "received_at": evt.ReceivedAt,
	})
	for _, sub := range *subs {
		select {
		case sub.ch <- data:
		default: // Drop if full
		}
	}
}

// --- Export (Pro) ---

func (s *Server) handleExportEvents(w http.ResponseWriter, r *http.Request) {
	if !s.limits.ExportJSON {
		writeJSON(w, 402, map[string]string{"error": "JSON export requires Pro — upgrade at https://stockyard.dev/corral/", "upgrade": "https://stockyard.dev/corral/"})
		return
	}
	events, err := s.db.ListEvents(r.PathValue("id"), 10000)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="events.json"`)
	writeJSON(w, 200, map[string]any{"events": events, "count": len(events)})
}

// --- Status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.db.Stats())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func itoa(n int) string { return strconv.Itoa(n) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
