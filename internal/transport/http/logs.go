package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	tail, err := parseTail(r)
	if err != nil {
		writeServiceError(w, h.options.Logger, err)
		return
	}
	reader, err := h.tasks.Logs(r.Context(), r.PathValue("project"), r.PathValue("name"),
		core.LogOptions{Tail: tail, Timestamps: true})
	if err != nil {
		writeServiceError(w, h.options.Logger, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Get("download") == "true" {
		filename := safeFilename(r.PathValue("project")) + "-" + safeFilename(r.PathValue("name"))
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`-logs.txt"`)
	}
	if _, err := stdcopy.StdCopy(w, w, reader); err != nil && !errors.Is(err, r.Context().Err()) {
		h.options.Logger.Printf("stream task logs: %v", err)
	}
}

func safeFilename(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "task"
	}
	return b.String()
}

func (h *Handler) streamLogs(w http.ResponseWriter, r *http.Request) {
	tail, err := parseTail(r)
	if err != nil {
		writeServiceError(w, h.options.Logger, err)
		return
	}
	reader, err := h.tasks.Logs(r.Context(), r.PathValue("project"), r.PathValue("name"),
		core.LogOptions{Tail: tail, Follow: true, Timestamps: true})
	if err != nil {
		writeServiceError(w, h.options.Logger, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	entries := make(chan logEntry, 64)
	done := make(chan error, 1)
	go func() {
		stdout := &logLineWriter{done: r.Context().Done(), stream: "stdout", out: entries}
		stderr := &logLineWriter{done: r.Context().Done(), stream: "stderr", out: entries}
		_, copyErr := stdcopy.StdCopy(stdout, stderr, reader)
		stdout.Flush()
		stderr.Flush()
		done <- copyErr
	}()

	heartbeat := time.NewTicker(h.options.Heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case entry := <-entries:
			payload, _ := json.Marshal(entry)
			if _, err = w.Write(append(append([]byte("data: "), payload...), '\n', '\n')); err != nil {
				return
			}
			if err = rc.Flush(); err != nil {
				return
			}
		case err = <-done:
			// Drain entries queued before the decoder completed.
			for {
				select {
				case entry := <-entries:
					payload, _ := json.Marshal(entry)
					_, _ = w.Write(append(append([]byte("data: "), payload...), '\n', '\n'))
				default:
					if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, r.Context().Err()) {
						h.options.Logger.Printf("decode task log stream: %v", err)
					}
					_ = rc.Flush()
					return
				}
			}
		case <-heartbeat.C:
			if _, err = io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err = rc.Flush(); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
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
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			return len(p), nil
		}
		line := string(w.buffer[:index])
		w.buffer = w.buffer[index+1:]
		if err := w.emit(strings.TrimSuffix(line, "\r")); err != nil {
			return 0, err
		}
	}
}

func (w *logLineWriter) Flush() {
	if len(w.buffer) > 0 {
		_ = w.emit(strings.TrimSuffix(string(w.buffer), "\r"))
		w.buffer = nil
	}
}

func (w *logLineWriter) emit(line string) error {
	timestamp := time.Now().UTC()
	message := line
	if first, rest, found := strings.Cut(line, " "); found {
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
