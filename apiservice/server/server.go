// Copyright © 2019 Xavier Basty <xbasty@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apex/log"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/go-chi/valve"
	"github.com/pkg/errors"
	"github.com/xav/f3/events"
	"gopkg.in/mgo.v2/bson"
)

type Server struct {
	Router        *chi.Mux
	Port          int
	Nats          NatsConn
	routesHandler RoutesHandler
	ctx           context.Context
}

const paymentURLParam = "payment"

// NewServer returns a valid Server.
func NewServer(options ...func(s *Server) error) (*Server, error) {
	r := chi.NewRouter()
	r.Use(
		render.SetContentType(render.ContentTypeJSON),
		middleware.Logger,
		middleware.RedirectSlashes,
		middleware.Recoverer,
	)

	s := &Server{
		Router: r,
	}
	s.routesHandler = s

	for _, o := range options {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	if err := s.routes(s.Router); err != nil {
		return nil, errors.Wrap(err, "failed to install routes")
	}

	return s, nil
}

// Start creates the routes and starts serving traffic.
func (s *Server) Start() error {
	vlv := valve.New()
	s.ctx = vlv.Context()

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: s.Router,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		_ = <-stop
		log.Info("shutting down..")

		// TODO: Drain NATS Connections

		// first valve
		if err := vlv.Shutdown(20 * time.Second); err != nil {
			log.WithError(err).Error("valve shutdown failed")
		}

		// create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// start http shutdown
		if err := srv.Shutdown(ctx); err != nil {
			log.WithError(err).Error("http server shutdown failed")
		}

		// verify, in worst case call cancel via defer
		select {
		case <-time.After(21 * time.Second):
			log.Warn("not all connections were closed.")
		case <-ctx.Done():
		}
	}()

	log.Infof("listening on :%v ...", s.Port)
	log.Infof("%v", srv.ListenAndServe())

	return nil
}

func (s *Server) routes(r *chi.Mux) error {
	r.Get("/", s.routesHandler.ListVersions) // GET  /

	// /v1/
	r.Route("/v1", func(r chi.Router) {
		r.Get("/", s.routesHandler.ListPayments)   // GET  /
		r.Post("/", s.routesHandler.CreatePayment) // POST /
		r.Put("/", s.routesHandler.UpdatePayment)  // PUT  /

		r.Route("/{"+paymentURLParam+"}", func(r chi.Router) {
			r.Get("/", s.routesHandler.FetchPayment)     // GET    /{payment}
			r.Delete("/", s.routesHandler.DeletePayment) // DELETE /{payment}
		})
	})

	return nil
}

//go:generate mockery -name RoutesHandler

type RoutesHandler interface {
	ListVersions(w http.ResponseWriter, r *http.Request)
	ListPayments(w http.ResponseWriter, r *http.Request)
	CreatePayment(w http.ResponseWriter, r *http.Request)
	FetchPayment(w http.ResponseWriter, r *http.Request)
	UpdatePayment(w http.ResponseWriter, r *http.Request)
	DeletePayment(w http.ResponseWriter, r *http.Request)
}

func (s *Server) ListVersions(w http.ResponseWriter, r *http.Request) {
	render.DefaultResponder(w, r, []string{"v1/"})
}

func (s *Server) ListPayments(w http.ResponseWriter, r *http.Request) {
	render.DefaultResponder(w, r, "ListPayments")
}

func (s *Server) CreatePayment(w http.ResponseWriter, r *http.Request) {
	payment := Payment{}

	if err := json.NewDecoder(r.Body).Decode(&payment); err != nil {
		log.WithError(err).Warnf("invalid request: '%v'", r.Body)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := payment.Validate(); err != nil {
		log.WithError(err).Warnf("invalid request: '%v'", r.Body)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := bson.Marshal(payment)
	if err != nil {
		log.WithError(err).Error("failed to marshal payment data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Nats.Publish(string(events.CreatePaymentEvent), data); err != nil {
		log.WithError(err).Error("Failed to publish create event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, "CreatePayment")
}

func (s *Server) FetchPayment(w http.ResponseWriter, r *http.Request) {
	render.DefaultResponder(w, r, "FetchPayment")
}

func (s *Server) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	payment := Payment{}

	if err := json.NewDecoder(r.Body).Decode(&payment); err != nil {
		log.WithError(err).Warnf("invalid request: '%v'", r.Body)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := payment.Validate(); err != nil {
		log.WithError(err).Warnf("invalid request: '%v'", r.Body)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := bson.Marshal(payment)
	if err != nil {
		log.WithError(err).Error("failed to marshal payment data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Nats.Publish(string(events.UpdatePaymentEvent), data); err != nil {
		log.WithError(err).Error("Failed to publish create event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, "CreatePayment")
}

func (s *Server) DeletePayment(w http.ResponseWriter, r *http.Request) {
	render.DefaultResponder(w, r, "DeletePayment")
}
