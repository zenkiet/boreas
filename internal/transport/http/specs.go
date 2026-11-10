package httptransport

import (
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

type startSpecRequest struct {
	Key      string   `json:"key" minLength:"1" maxLength:"63" example:"wo-718"`
	Projects []string `json:"projects" minItems:"1" maxItems:"5"`
	Ticket   string   `json:"ticket" minLength:"1" maxLength:"20000"`
}

type specAnswersRequest struct {
	ID      uuid.UUID         `json:"-" path:"id"`
	Round   int               `json:"round"`
	Answers []core.SpecAnswer `json:"answers"`
	Note    string            `json:"note,omitempty" maxLength:"2000"`
}

type specsQuery struct {
	Project string `query:"project"`
	Key     string `query:"key"`
}

type specExportQuery struct {
	ID      uuid.UUID `json:"-" path:"id"`
	Project string    `query:"project" required:"true"`
}

type specItemDTO struct {
	ID        uuid.UUID      `json:"id"`
	Key       string         `json:"key"`
	Title     string         `json:"title"`
	Projects  []string       `json:"projects"`
	State     core.SpecState `json:"state"`
	Questions int            `json:"questions"`
	CreatedBy *uuid.UUID     `json:"created_by"`
	Author    string         `json:"created_by_name,omitempty"`
	Stopped   core.SpecState `json:"stopped,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type specDTO struct {
	specItemDTO
	core.SpecBody
}

type specsResponse struct {
	Specs []specItemDTO `json:"specs"`
}

type specResponse struct {
	Spec specDTO `json:"spec"`
}

func specItemFromCore(s core.BoraSpec) specItemDTO {
	item := specItemDTO{
		ID: s.ID, Key: s.Key, Title: s.Title, Projects: s.Slugs, State: s.Status(),
		CreatedBy: s.CreatedBy, Author: s.CreatedByName, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	if item.State == core.SpecFailed {
		item.Stopped = s.State
	}
	if n := len(s.Body.Rounds); item.State == core.SpecAsking && n > 0 {
		item.Questions = len(s.Body.Rounds[n-1].Questions)
	}
	return item
}

func (h *Handler) startSpec(w http.ResponseWriter, r *http.Request) {
	var req startSpecRequest
	if decode(w, r, &req) {
		h.replySpec(w, http.StatusAccepted)(h.projects.StartSpec(r.Context(), userFrom(r.Context()), req.Key, req.Projects, req.Ticket))
	}
}

func (h *Handler) answerSpec(w http.ResponseWriter, r *http.Request) {
	var req specAnswersRequest
	if id, ok := pathID(w, r); ok && decode(w, r, &req) {
		h.replySpec(w, http.StatusAccepted)(h.projects.AnswerSpec(r.Context(), userFrom(r.Context()), id, req.Round, req.Answers, req.Note))
	}
}

func (h *Handler) writeSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.replySpec(w, http.StatusAccepted)(h.projects.WriteSpec(r.Context(), userFrom(r.Context()), id))
	}
}

func (h *Handler) retrySpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.replySpec(w, http.StatusAccepted)(h.projects.RetrySpec(r.Context(), userFrom(r.Context()), id))
	}
}

func (h *Handler) getSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.replySpec(w, http.StatusOK)(h.projects.BoraSpec(r.Context(), userFrom(r.Context()), id))
	}
}

func (h *Handler) replySpec(w http.ResponseWriter, status int) func(core.BoraSpec, error) {
	return func(spec core.BoraSpec, err error) {
		h.reply(w, status, specResponse{Spec: specDTO{specItemFromCore(spec), spec.Body}}, err)
	}
}

func (h *Handler) listSpecs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	specs, err := h.projects.BoraSpecs(r.Context(), userFrom(r.Context()), q.Get("project"), q.Get("key"))
	h.reply(w, http.StatusOK, specsResponse{Specs: convert(specs, specItemFromCore)}, err)
}

func (h *Handler) followSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		if events, err := h.projects.FollowSpec(r.Context(), userFrom(r.Context()), id); events != nil {
			sse(w, r, events, h.specEvent)
		} else {
			h.noContent(w, err)
		}
	}
}

func (h *Handler) stopSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.noContent(w, h.projects.StopSpec(r.Context(), userFrom(r.Context()), id))
	}
}

func (h *Handler) deleteSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.noContent(w, h.projects.DeleteSpec(r.Context(), userFrom(r.Context()), id))
	}
}

func (h *Handler) exportSpec(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		text, err := h.projects.ExportSpec(r.Context(), userFrom(r.Context()), id, r.URL.Query().Get("project"), origin(r))
		if err != nil {
			writeServiceError(w, h.logger, err)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, text)
	}
}

func (h *Handler) specEvent(e service.BoraEvent) (string, any) {
	switch e.Name {
	case "step":
		return e.Name, map[string]any{"key": e.Key, "text": e.Text, "done": e.Finished}
	case "done":
		return e.Name, struct{}{}
	}
	return h.boraEvent(e)
}
