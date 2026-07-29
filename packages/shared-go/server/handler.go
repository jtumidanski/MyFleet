package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

type Server struct {
	log    logrus.FieldLogger
	router chi.Router
	inits  []func(chi.Router)
}

func New(log logrus.FieldLogger) *Server {
	return &Server{log: log, router: chi.NewRouter()}
}

func (s *Server) Logger() logrus.FieldLogger { return s.log }

func (s *Server) Use(mw ...func(http.Handler) http.Handler) *Server {
	s.router.Use(mw...)
	return s
}

func (s *Server) AddRouteInitializer(fn func(chi.Router)) *Server {
	s.inits = append(s.inits, fn)
	return s
}

func (s *Server) Router() chi.Router {
	for _, fn := range s.inits {
		fn(s.router)
	}
	s.inits = nil
	return s.router
}

// RegisterInputHandler decodes a typed JSON:API attributes payload {data:{attributes:T}}.
func RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var doc struct {
			Data struct {
				Attributes T `json:"attributes"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			WriteError(w, ErrValidation)
			return
		}
		fn(w, r, doc.Data.Attributes)
	}
}
