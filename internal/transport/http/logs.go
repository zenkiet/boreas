package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/zenkiet/boreas/internal/core"
)

type logEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

const heartbeatInterval = 15 * time.Second

// sse frames each event as JSON data with a heartbeat while idle; true means events closed.
func sse[T any](w http.ResponseWriter, r *http.Request, events <-chan T, encode func(T) any) bool {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		frame := ": heartbeat\n\n"
		select {
		case event, ok := <-events:
			if !ok {
				return true
			}
			payload, _ := json.Marshal(encode(event))
			frame = "data: " + string(payload) + "\n\n"
		case <-heartbeat.C:
		case <-r.Context().Done():
			return false
		}
		if _, err := io.WriteString(w, frame); err != nil || rc.Flush() != nil {
			return false
		}
	}
}

func parseTail(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("tail")
	if raw == "" {
		return 100, nil
	}
	tail, err := strconv.Atoi(raw)
	if err != nil || tail < 0 {
		return 0, errors.Join(core.ErrInvalidInput, errors.New("tail must be a non-negative integer"))
	}
	return tail, nil
}

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	tail, err := parseTail(r)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	reader, err := h.tasks.Logs(r.Context(), r.PathValue("project"), r.PathValue("name"),
		core.LogOptions{Tail: tail, Timestamps: true})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("project")+"-"+r.PathValue("name")+`-logs.txt"`)
	}
	if _, err := stdcopy.StdCopy(w, w, reader); err != nil && !errors.Is(err, r.Context().Err()) {
		h.logger.Error("stream task logs", "error", err)
	}
}

func (h *Handler) streamLogs(w http.ResponseWriter, r *http.Request) {
	tail, err := parseTail(r)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	opts := core.LogOptions{Tail: tail, Follow: true, Timestamps: true}
	if raw := r.URL.Query().Get("since"); raw != "" {
		since, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeBadRequest(w)
			return
		}
		// Docker's since includes its own instant, so resuming starts one nanosecond later.
		// ponytail: two lines stamped in the same nanosecond can lose the second across a reconnect.
		opts.Since = since.Add(time.Nanosecond)
	}
	reader, err := h.tasks.Logs(r.Context(), r.PathValue("project"), r.PathValue("name"), opts)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	defer reader.Close()

	entries := make(chan logEntry, 64)
	var copyErr error
	go func() {
		defer close(entries)
		stdout := &logLineWriter{done: r.Context().Done(), stream: "stdout", out: entries}
		stderr := &logLineWriter{done: r.Context().Done(), stream: "stderr", out: entries}
		_, copyErr = stdcopy.StdCopy(stdout, stderr, reader)
		stdout.Flush()
		stderr.Flush()
	}()
	if sse(w, r, entries, func(e logEntry) any { return e }) && copyErr != nil &&
		!errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, r.Context().Err()) {
		h.logger.Error("decode task log stream", "error", copyErr)
	}
}

type logLineWriter struct {
	done   <-chan struct{}
	stream string
	out    chan<- logEntry
	buffer []byte
}

func (w *logLineWriter) Write(p []byte) (int, error) {
	w.buffer = append(w.buffer, p...)
	for {
		line, rest, found := bytes.Cut(w.buffer, []byte{'\n'})
		if !found {
			return len(p), nil
		}
		w.buffer = rest
		if err := w.emit(string(line)); err != nil {
			return 0, err
		}
	}
}

func (w *logLineWriter) Flush() {
	if len(w.buffer) > 0 {
		_ = w.emit(string(w.buffer))
		w.buffer = nil
	}
}

func (w *logLineWriter) emit(line string) error {
	timestamp, message := time.Now().UTC(), strings.TrimSuffix(line, "\r")
	if first, rest, found := strings.Cut(message, " "); found {
		if parsed, err := time.Parse(time.RFC3339Nano, first); err == nil {
			timestamp, message = parsed, rest
		}
	}
	select {
	case w.out <- logEntry{Timestamp: timestamp, Stream: w.stream, Message: message}:
		return nil
	case <-w.done:
		return context.Canceled
	}
}
