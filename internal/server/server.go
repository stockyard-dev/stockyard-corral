package server

import (
	"context"
	"database/sql"
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

// Server is the Corral HTTP server.
type Server struct {
	db   *sql.DB
	mux  *http.ServeMux
	port int
	sse  *SSEBroadcaster
	srv  *http.Server
}

// SSEBroadcaster sends events to connected SSE clients.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan []byte]bool
}

func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{clients: make(map[chan []byte]bool)}
}

func (b *SSEBroadcaster) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.clients[ch] = true
	b.mu.Unlock()
	return ch
}

func (b *SSEBroadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *SSEBroadcaster) Broadcast(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// drop if client is slow
		}
	}
}

// New creates a new Corral server.
func New(db *sql.DB, port int) *Server {
	mux := http.NewServeMux()
	s := &Server{db: db, mux: mux, port: port, sse: NewSSEBroadcaster()}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Webhook capture (any method)
	s.mux.HandleFunc("/hook/", s.handleCapture)

	// API: Endpoints
	s.mux.HandleFunc("GET /api/endpoints", s.handleListEndpoints)
	s.mux.HandleFunc("POST /api/endpoints", s.handleCreateEndpoint)
	s.mux.HandleFunc("GET /api/endpoints/{id}", s.handleGetEndpoint)
	s.mux.HandleFunc("DELETE /api/endpoints/{id}", s.handleDeleteEndpoint)

	// API: Events
	s.mux.HandleFunc("GET /api/endpoints/{id}/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/events/{id}", s.handleGetEvent)
	s.mux.HandleFunc("DELETE /api/events/{id}", s.handleDeleteEvent)
	s.mux.HandleFunc("POST /api/events/{id}/replay", s.handleReplayEvent)

	// API: Forwards
	s.mux.HandleFunc("GET /api/endpoints/{id}/forwards", s.handleListForwards)
	s.mux.HandleFunc("POST /api/endpoints/{id}/forwards", s.handleCreateForward)
	s.mux.HandleFunc("DELETE /api/forwards/{id}", s.handleDeleteForward)

	// API: Deliveries
	s.mux.HandleFunc("GET /api/events/{id}/deliveries", s.handleListDeliveries)

	// SSE: Live event stream
	s.mux.HandleFunc("GET /api/live", s.handleSSE)

	// Health
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Version
	s.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		var endpoints, events int
		s.db.QueryRow("SELECT COUNT(*) FROM endpoints").Scan(&endpoints)
		s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&events)
		writeJSON(w, 200, map[string]any{
			"product":   "stockyard-corral",
			"version":   "0.1.0",
			"endpoints": endpoints,
			"events":    events,
		})
	})
}

// Start begins listening.
func (s *Server) Start() error {
	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	log.Printf("[corral] listening on :%d", s.port)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// ──────────────────────────────────────────────────────────────────────
// Webhook Capture
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	// Extract endpoint ID from path: /hook/{endpoint_id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hook/"), "/")
	endpointID := parts[0]
	if endpointID == "" {
		writeJSON(w, 400, map[string]string{"error": "missing endpoint ID"})
		return
	}

	// Verify endpoint exists
	var name string
	err := s.db.QueryRow("SELECT name FROM endpoints WHERE id = ?", endpointID).Scan(&name)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "endpoint not found"})
		return
	}

	// Read body (limit to 1MB)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "failed to read body"})
		return
	}

	// Capture headers
	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}
	headersJSON, _ := json.Marshal(headers)

	// Store event
	eventID := store.GenerateID("evt_")
	sourceIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		sourceIP = strings.Split(forwarded, ",")[0]
	}
	subPath := "/" + strings.Join(parts[1:], "/")

	_, err = s.db.Exec(`INSERT INTO events (id, endpoint_id, method, path, headers_json, body, body_size, content_type, source_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, endpointID, r.Method, subPath, string(headersJSON), string(body), len(body),
		r.Header.Get("Content-Type"), sourceIP)
	if err != nil {
		log.Printf("[capture] insert error: %v", err)
		writeJSON(w, 500, map[string]string{"error": "storage error"})
		return
	}

	log.Printf("[capture] %s %s → %s (%s, %d bytes)", r.Method, endpointID, eventID, r.Header.Get("Content-Type"), len(body))

	// Broadcast to SSE clients
	evt, _ := json.Marshal(map[string]any{
		"event_id":     eventID,
		"endpoint_id":  endpointID,
		"endpoint":     name,
		"method":       r.Method,
		"path":         subPath,
		"content_type": r.Header.Get("Content-Type"),
		"body_size":    len(body),
		"source_ip":    sourceIP,
		"received_at":  time.Now().UTC().Format(time.RFC3339),
	})
	s.sse.Broadcast(evt)

	// Trigger forwards (async)
	go s.runForwards(eventID, endpointID, r.Method, headersJSON, body)

	// Return 200 to the webhook sender
	writeJSON(w, 200, map[string]any{"status": "captured", "event_id": eventID})
}

// ──────────────────────────────────────────────────────────────────────
// Endpoints API
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT e.id, e.name, e.description, e.created_at,
		(SELECT COUNT(*) FROM events WHERE endpoint_id = e.id) as event_count
		FROM endpoints e ORDER BY e.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var endpoints []map[string]any
	for rows.Next() {
		var id, name, desc, created string
		var count int
		if rows.Scan(&id, &name, &desc, &created, &count) != nil {
			continue
		}
		endpoints = append(endpoints, map[string]any{
			"id": id, "name": name, "description": desc,
			"url": fmt.Sprintf("/hook/%s", id), "event_count": count,
			"created_at": created,
		})
	}
	if endpoints == nil {
		endpoints = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"endpoints": endpoints, "count": len(endpoints)})
}

func (s *Server) handleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" {
		req.Name = "endpoint-" + time.Now().Format("20060102-150405")
	}

	id := store.GenerateID("ep_")
	_, err := s.db.Exec("INSERT INTO endpoints (id, name, description) VALUES (?, ?, ?)", id, req.Name, req.Description)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 201, map[string]any{
		"id":   id,
		"name": req.Name,
		"url":  fmt.Sprintf("/hook/%s", id),
	})
}

func (s *Server) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, desc, created string
	err := s.db.QueryRow("SELECT name, description, created_at FROM endpoints WHERE id = ?", id).Scan(&name, &desc, &created)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "endpoint not found"})
		return
	}
	var eventCount, forwardCount int
	s.db.QueryRow("SELECT COUNT(*) FROM events WHERE endpoint_id = ?", id).Scan(&eventCount)
	s.db.QueryRow("SELECT COUNT(*) FROM forwards WHERE endpoint_id = ?", id).Scan(&forwardCount)

	writeJSON(w, 200, map[string]any{
		"id": id, "name": name, "description": desc,
		"url": fmt.Sprintf("/hook/%s", id), "event_count": eventCount,
		"forward_count": forwardCount, "created_at": created,
	})
}

func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.db.Exec("DELETE FROM deliveries WHERE event_id IN (SELECT id FROM events WHERE endpoint_id = ?)", id)
	s.db.Exec("DELETE FROM events WHERE endpoint_id = ?", id)
	s.db.Exec("DELETE FROM forwards WHERE endpoint_id = ?", id)
	s.db.Exec("DELETE FROM endpoints WHERE id = ?", id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────
// Events API
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	endpointID := r.PathValue("id")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	rows, err := s.db.Query(`SELECT id, method, path, content_type, body_size, source_ip, received_at
		FROM events WHERE endpoint_id = ? ORDER BY received_at DESC LIMIT ?`, endpointID, limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var id, method, path, ct, ip, received string
		var size int
		if rows.Scan(&id, &method, &path, &ct, &size, &ip, &received) != nil {
			continue
		}
		events = append(events, map[string]any{
			"id": id, "method": method, "path": path, "content_type": ct,
			"body_size": size, "source_ip": ip, "received_at": received,
		})
	}
	if events == nil {
		events = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"events": events, "count": len(events)})
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var endpointID, method, path, headersJSON, body, ct, ip, received string
	var size int
	err := s.db.QueryRow(`SELECT endpoint_id, method, path, headers_json, body, body_size, content_type, source_ip, received_at
		FROM events WHERE id = ?`, id).Scan(&endpointID, &method, &path, &headersJSON, &body, &size, &ct, &ip, &received)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}

	var headers any
	json.Unmarshal([]byte(headersJSON), &headers)

	writeJSON(w, 200, map[string]any{
		"id": id, "endpoint_id": endpointID, "method": method, "path": path,
		"headers": headers, "body": body, "body_size": size,
		"content_type": ct, "source_ip": ip, "received_at": received,
	})
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.db.Exec("DELETE FROM deliveries WHERE event_id = ?", id)
	s.db.Exec("DELETE FROM events WHERE id = ?", id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────
// Replay
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleReplayEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("id")

	var req struct {
		Target string `json:"target"` // URL to replay to
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
		writeJSON(w, 400, map[string]string{"error": "target URL required"})
		return
	}

	// Load original event
	var method, headersJSON, body, ct string
	err := s.db.QueryRow(`SELECT method, headers_json, body, content_type FROM events WHERE id = ?`, eventID).
		Scan(&method, &headersJSON, &body, &ct)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}

	// Replay
	result := s.deliverWebhook(eventID, req.Target, method, []byte(headersJSON), []byte(body), ct)

	writeJSON(w, 200, result)
}

// ──────────────────────────────────────────────────────────────────────
// Forwards
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleListForwards(w http.ResponseWriter, r *http.Request) {
	endpointID := r.PathValue("id")
	rows, err := s.db.Query("SELECT id, target_url, enabled, retry_count, created_at FROM forwards WHERE endpoint_id = ?", endpointID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var forwards []map[string]any
	for rows.Next() {
		var id, retry int
		var target, created string
		var enabled int
		if rows.Scan(&id, &target, &enabled, &retry, &created) != nil {
			continue
		}
		forwards = append(forwards, map[string]any{
			"id": id, "target_url": target, "enabled": enabled == 1,
			"retry_count": retry, "created_at": created,
		})
	}
	if forwards == nil {
		forwards = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"forwards": forwards})
}

func (s *Server) handleCreateForward(w http.ResponseWriter, r *http.Request) {
	endpointID := r.PathValue("id")
	var req struct {
		TargetURL  string `json:"target_url"`
		RetryCount int    `json:"retry_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetURL == "" {
		writeJSON(w, 400, map[string]string{"error": "target_url required"})
		return
	}
	if req.RetryCount <= 0 {
		req.RetryCount = 3
	}

	res, err := s.db.Exec("INSERT INTO forwards (endpoint_id, target_url, retry_count) VALUES (?, ?, ?)",
		endpointID, req.TargetURL, req.RetryCount)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]any{"id": id, "target_url": req.TargetURL, "retry_count": req.RetryCount})
}

func (s *Server) handleDeleteForward(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.db.Exec("DELETE FROM forwards WHERE id = ?", id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────
// Deliveries
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("id")
	rows, err := s.db.Query(`SELECT target_url, status_code, success, error_message, latency_ms, attempt, created_at
		FROM deliveries WHERE event_id = ? ORDER BY created_at DESC`, eventID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var deliveries []map[string]any
	for rows.Next() {
		var target, errMsg, created string
		var code, latency, attempt, success int
		if rows.Scan(&target, &code, &success, &errMsg, &latency, &attempt, &created) != nil {
			continue
		}
		deliveries = append(deliveries, map[string]any{
			"target_url": target, "status_code": code, "success": success == 1,
			"error": errMsg, "latency_ms": latency, "attempt": attempt, "created_at": created,
		})
	}
	if deliveries == nil {
		deliveries = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"deliveries": deliveries})
}

// ──────────────────────────────────────────────────────────────────────
// SSE Live Stream
// ──────────────────────────────────────────────────────────────────────

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.sse.Subscribe()
	defer s.sse.Unsubscribe(ch)

	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// ──────────────────────────────────────────────────────────────────────
// Forwarding Engine
// ──────────────────────────────────────────────────────────────────────

func (s *Server) runForwards(eventID, endpointID, method string, headersJSON, body []byte) {
	rows, err := s.db.Query("SELECT target_url, retry_count FROM forwards WHERE endpoint_id = ? AND enabled = 1", endpointID)
	if err != nil {
		return
	}
	defer rows.Close()

	var ct string
	var headers map[string]string
	json.Unmarshal(headersJSON, &headers)
	ct = headers["Content-Type"]

	for rows.Next() {
		var target string
		var retries int
		if rows.Scan(&target, &retries) != nil {
			continue
		}
		go func(t string, r int) {
			s.deliverWithRetry(eventID, t, method, body, ct, r)
		}(target, retries)
	}
}

func (s *Server) deliverWithRetry(eventID, target, method string, body []byte, ct string, maxRetries int) {
	backoff := 1 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result := s.deliverWebhook(eventID, target, method, nil, body, ct)
		if result["success"] == true {
			return
		}
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
}

func (s *Server) deliverWebhook(eventID, target, method string, headersJSON, body []byte, ct string) map[string]any {
	if method == "" {
		method = "POST"
	}
	if ct == "" {
		ct = "application/json"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(method, target, strings.NewReader(string(body)))
	if err != nil {
		result := map[string]any{"success": false, "error": err.Error()}
		s.recordDelivery(eventID, target, 0, false, err.Error(), 0, 1)
		return result
	}
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-Corral-Event-ID", eventID)
	req.Header.Set("User-Agent", "Stockyard-Corral/0.1")

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		result := map[string]any{"success": false, "error": err.Error(), "latency_ms": latency}
		s.recordDelivery(eventID, target, 0, false, err.Error(), int(latency), 1)
		return result
	}
	resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	errMsg := ""
	if !success {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	s.recordDelivery(eventID, target, resp.StatusCode, success, errMsg, int(latency), 1)

	return map[string]any{
		"success":     success,
		"status_code": resp.StatusCode,
		"latency_ms":  latency,
	}
}

func (s *Server) recordDelivery(eventID, target string, code int, success bool, errMsg string, latency, attempt int) {
	successInt := 0
	if success {
		successInt = 1
	}
	s.db.Exec(`INSERT INTO deliveries (event_id, target_url, status_code, success, error_message, latency_ms, attempt)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, eventID, target, code, successInt, errMsg, latency, attempt)
}

// ──────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
