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
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/models"
	"gopkg.in/mgo.v2/bson"
)

type Server struct {
	Router        *chi.Mux
	Port          int
	Nats          f3nats.NatsConn
	routesHandler RoutesHandler
	ctx           context.Context
}

const (
	organisationURLParam = "organisation"
	paymentURLParam      = "payment"
)

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

	log.Infof("f3 payment api listening on :%v ...", s.Port)
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

		r.Route("/{"+organisationURLParam+"}/{"+paymentURLParam+"}", func(r chi.Router) {
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
	data, err := bson.Marshal(models.ResourceLocator{
		OrganisationID: nil,
		ID:             nil,
	})
	if err != nil {
		log.WithError(err).Error("failed to marshal payment resource locator")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reply, err := s.Nats.Request(string(models.ListPaymentEvent), data, 10*time.Millisecond)
	if err != nil {
		log.WithError(err).Error("list request event failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	replyEvent := models.PaymentListEvent{}
	if err := bson.Unmarshal(reply.Data, &replyEvent); err != nil {
		log.WithError(err).Error("failed to decode reply data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		render.DefaultResponder(w, r, replyEvent.Resource)
	case models.ServiceErrorEvent:
		replyEvent := models.Event{}
		_ = bson.Unmarshal(reply.Data, &replyEvent)
		cause := replyEvent.Resource.(bson.M)["cause"].(string)
		http.Error(w, fmt.Sprintf(cause), http.StatusInternalServerError)
	default:
		log.Errorf("unrecognised response to list request: '%v'", replyEvent.EventType)
		http.Error(w, fmt.Sprintf("unrecognised response to list request: '%v'", replyEvent.EventType), http.StatusInternalServerError)
	}
}

func (s *Server) CreatePayment(w http.ResponseWriter, r *http.Request) {
	payment := models.Payment{}
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

	if err := s.Nats.Publish(string(models.CreatePaymentEvent), data); err != nil {
		log.WithError(err).Error("Failed to publish create event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, "CreatePayment")
}

func (s *Server) FetchPayment(w http.ResponseWriter, r *http.Request) {
	oid, err := uuid.Parse(chi.URLParam(r, organisationURLParam))
	if err != nil {
		log.WithError(err).Warnf("invalid organisation id: '%v'", chi.URLParam(r, organisationURLParam))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, paymentURLParam))
	if err != nil {
		log.WithError(err).Warnf("invalid resource id: '%v'", chi.URLParam(r, organisationURLParam))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	data, err := bson.Marshal(models.ResourceLocator{
		OrganisationID: &oid,
		ID:             &pid,
	})
	if err != nil {
		log.WithError(err).Error("failed to marshal payment resource locator")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg, err := s.Nats.Request(string(models.FetchPaymentEvent), data, 10*time.Millisecond)
	if err != nil {
		log.WithError(err).Error("fetch request event failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	replyEvent := models.PaymentEvent{}
	if err := bson.Unmarshal(msg.Data, &replyEvent); err != nil {
		log.WithError(err).Error("failed to decode reply data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		render.DefaultResponder(w, r, replyEvent.Resource)
	case models.ResourceNotFoundEvent:
		log.Warnf("payment '%v/%v' was not found", oid.String(), pid.String())
		http.Error(w, fmt.Sprintf("payment '%v/%v' was not found", oid.String(), pid.String()), http.StatusNotFound)
	case models.ServiceErrorEvent:
		replyEvent := models.Event{}
		_ = bson.Unmarshal(msg.Data, &replyEvent)
		cause := replyEvent.Resource.(bson.M)["cause"].(string)
		http.Error(w, fmt.Sprintf(cause), http.StatusInternalServerError)
	default:
		log.Errorf("unrecognised response to fetch request: '%v'", replyEvent.EventType)
		http.Error(w, fmt.Sprintf("unrecognised response to fetch request: '%v'", replyEvent.EventType), http.StatusInternalServerError)
	}
}

func (s *Server) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	payment := models.Payment{}

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

	if err := s.Nats.Publish(string(models.UpdatePaymentEvent), data); err != nil {
		log.WithError(err).Error("failed to publish create event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, "CreatePayment")
}

func (s *Server) DeletePayment(w http.ResponseWriter, r *http.Request) {
	oid, err := uuid.Parse(chi.URLParam(r, organisationURLParam))
	if err != nil {
		log.WithError(err).Warnf("invalid organisation id: '%v'", chi.URLParam(r, organisationURLParam))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, paymentURLParam))
	if err != nil {
		log.WithError(err).Warnf("invalid resource id: '%v'", chi.URLParam(r, organisationURLParam))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	data, err := bson.Marshal(models.ResourceLocator{
		OrganisationID: &oid,
		ID:             &pid,
	})
	if err != nil {
		log.WithError(err).Error("failed to marshal resource resource locator")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg, err := s.Nats.Request(string(models.DeletePaymentEvent), data, 10*time.Millisecond)
	if err != nil {
		log.WithError(err).Error("fetch request event failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	replyEvent := models.LocatorEvent{}
	if err := bson.Unmarshal(msg.Data, &replyEvent); err != nil {
		log.WithError(err).Error("failed to decode reply data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		render.Status(r, http.StatusGone)
		render.DefaultResponder(w, r, replyEvent.Resource)
	case models.ResourceNotFoundEvent:
		log.Warnf("resource '%v/%v' was not found", oid.String(), pid.String())
		http.Error(w, fmt.Sprintf("resource '%v/%v' was not found", oid.String(), pid.String()), http.StatusNotFound)
	case models.ServiceErrorEvent:
		replyEvent := models.Event{}
		_ = bson.Unmarshal(msg.Data, &replyEvent)
		cause := replyEvent.Resource.(bson.M)["cause"].(string)
		http.Error(w, fmt.Sprintf(cause), http.StatusInternalServerError)
	default:
		log.Errorf("unrecognised response to fetch request: '%v'", replyEvent.EventType)
		http.Error(w, fmt.Sprintf("unrecognised response to fetch request: '%v'", replyEvent.EventType), http.StatusInternalServerError)
	}
}
