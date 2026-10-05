package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type API struct {
	mgr   *SessionManager
	token string
}

func NewAPI(mgr *SessionManager, token string) *API { return &API{mgr: mgr, token: token} }

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}) })

	r.Group(func(r chi.Router) {
		r.Use(a.auth)
		r.Post("/sessions", a.createSession)
		r.Route("/sessions/{id}", func(r chi.Router) {
			r.Get("/status", a.withSession(a.status))
			r.Get("/qr", a.withSession(a.qr))
			r.Post("/pair-phone", a.withSession(a.pairPhone))
			r.Post("/reconnect", a.withSession(a.reconnect))
			r.Delete("/", a.deleteSession)
			r.Post("/messages", a.withSession(a.sendMessage))
			r.Post("/reactions", a.withSession(a.react))
			r.Post("/read", a.withSession(a.markRead))
			r.Get("/media/{msgID}", a.withSession(a.media))
			r.Get("/groups/{jid}", a.withSession(a.group))
			r.Get("/contacts/{jid}/avatar", a.withSession(a.avatar))
		})
	})
	return r
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type sessionHandler func(http.ResponseWriter, *http.Request, *Session)

func (a *API) withSession(h sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := a.mgr.Get(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		h(w, r, s)
	}
}

func (a *API) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID            string `json:"session_id"`
		WebhookURL    string `json:"webhook_url"`
		WebhookSecret string `json:"webhook_secret"`
		ImportHistory *bool  `json:"import_history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.WebhookURL == "" || body.WebhookSecret == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("session_id, webhook_url and webhook_secret are required"))
		return
	}
	rec := SessionRecord{ID: body.ID, WebhookURL: body.WebhookURL, WebhookSecret: body.WebhookSecret, ImportHistory: true}
	if body.ImportHistory != nil {
		rec.ImportHistory = *body.ImportHistory
	}
	s, err := a.mgr.Create(r.Context(), rec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.Status())
}

func (a *API) status(w http.ResponseWriter, _ *http.Request, s *Session) {
	writeJSON(w, http.StatusOK, s.Status())
}

func (a *API) qr(w http.ResponseWriter, r *http.Request, s *Session) {
	code, err := s.StartPairing(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"qr": code})
}

func (a *API) pairPhone(w http.ResponseWriter, r *http.Request, s *Session) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phone == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("phone is required"))
		return
	}
	code, err := s.PairPhone(r.Context(), strings.TrimPrefix(body.Phone, "+"))
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (a *API) reconnect(w http.ResponseWriter, _ *http.Request, s *Session) {
	if err := s.Reconnect(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, s.Status())
}

func (a *API) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := a.mgr.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) sendMessage(w http.ResponseWriter, r *http.Request, s *Session) {
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("invalid message request"))
		return
	}
	res, err := s.Send(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) react(w http.ResponseWriter, r *http.Request, s *Session) {
	var body struct {
		To        string `json:"to"`
		Sender    string `json:"sender"`
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" || body.MessageID == "" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("to and message_id are required"))
		return
	}
	id, err := s.React(r.Context(), body.To, body.Sender, body.MessageID, body.Emoji)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, SendResult{ID: id})
}

func (a *API) markRead(w http.ResponseWriter, r *http.Request, s *Session) {
	var body struct {
		Chat   string   `json:"chat"`
		Sender string   `json:"sender"`
		IDs    []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Chat == "" || len(body.IDs) == 0 {
		writeError(w, http.StatusUnprocessableEntity, errors.New("chat and ids are required"))
		return
	}
	if err := s.MarkRead(r.Context(), body.Chat, body.Sender, body.IDs); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) media(w http.ResponseWriter, r *http.Request, s *Session) {
	data, mime, filename, err := s.Media(r.Context(), chi.URLParam(r, "msgID"))
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	} else if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", mime)
	if filename != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filename, `"`, "")+`"`)
	}
	_, _ = w.Write(data)
}

func (a *API) group(w http.ResponseWriter, r *http.Request, s *Session) {
	info, err := s.GroupInfo(r.Context(), chi.URLParam(r, "jid"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": Identifier(info.JID), "subject": info.Name, "participants": len(info.Participants)})
}

func (a *API) avatar(w http.ResponseWriter, r *http.Request, s *Session) {
	url, err := s.Avatar(r.Context(), chi.URLParam(r, "jid"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
