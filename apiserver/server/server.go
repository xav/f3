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
	"github.com/buger/jsonparser"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/go-chi/valve"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/models"
)

type Server struct {
	Router        *chi.Mux
	Port          int
	Nats          f3nats.NatsConn
	routesHandler RoutesHandler
	ctx           context.Context
	natsTimeout   time.Duration
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
		Router:      r,
		natsTimeout: 100 * time.Millisecond,
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
			r.Get("/", s.routesHandler.FetchPayment)     // GET    /{organisationID}/{paymentID}
			r.Delete("/", s.routesHandler.DeletePayment) // DELETE /{organisationID}/{paymentID}
			r.Post("/dump", s.routesHandler.DumpPayment) // POST   /{organisationID}/{paymentID}/dump
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
	DumpPayment(w http.ResponseWriter, r *http.Request)
}

func (s *Server) ListVersions(w http.ResponseWriter, r *http.Request) {
	render.DefaultResponder(w, r, []string{"v1/"})
}

func (s *Server) ListPayments(w http.ResponseWriter, r *http.Request) {
	rt := models.PaymentResource
	data, err := json.Marshal(models.ResourceLocator{
		ResourceType:   &rt,
		OrganisationID: nil,
		ID:             nil,
	})
	if err != nil {
		logError(w, err, "failed to encode payment resource locator")
		return
	}

	reply, err := s.Nats.Request(string(models.ListPaymentEvent), data, s.natsTimeout)
	if err != nil {
		logError(w, err, "list request event failed")
		return
	}

	replyEvent := models.Event{}
	if err := json.Unmarshal(reply.Data, &replyEvent); err != nil {
		logError(w, err, "failed to decode reply data")
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		payments := make([]models.Payment, 0)
		if err := json.Unmarshal([]byte(replyEvent.Resource), &payments); err != nil {
			logError(w, err, "failed to decode payments data")
			return
		}
		render.DefaultResponder(w, r, payments)
	case models.ServiceErrorEvent:
		if cause, err := jsonparser.GetString([]byte(replyEvent.Resource), "cause"); err != nil {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		} else {
			http.Error(w, cause, http.StatusInternalServerError)
		}
	default:
		logErrorf(w, nil, "unrecognised response to list request: '%v'", replyEvent.EventType)
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

	data, err := json.Marshal(payment)
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
	render.DefaultResponder(w, r, "Accepted")
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

	rt := models.PaymentResource
	data, err := json.Marshal(models.ResourceLocator{
		ResourceType:   &rt,
		OrganisationID: &oid,
		ID:             &pid,
	})
	if err != nil {
		logError(w, err, "failed to encode payment resource locator")
		return
	}

	msg, err := s.Nats.Request(string(models.FetchPaymentEvent), data, s.natsTimeout)
	if err != nil {
		logError(w, err, "fetch request event failed")
		return
	}

	replyEvent := models.Event{}
	if err := json.Unmarshal(msg.Data, &replyEvent); err != nil {
		logError(w, err, "failed to decode reply data")
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		payment := models.Payment{}
		if err := json.Unmarshal([]byte(replyEvent.Resource), &payment); err != nil {
			logError(w, err, "failed to decode payment data")
			return
		}
		render.DefaultResponder(w, r, payment)
	case models.ResourceNotFoundEvent:
		log.Warnf("payment '%v/%v' was not found", oid.String(), pid.String())
		http.Error(w, fmt.Sprintf("payment '%v/%v' was not found", oid.String(), pid.String()), http.StatusNotFound)
	case models.ServiceErrorEvent:
		if cause, err := jsonparser.GetString([]byte(replyEvent.Resource), "cause"); err != nil {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		} else {
			http.Error(w, cause, http.StatusInternalServerError)
		}
	default:
		logErrorf(w, nil, "unrecognised response to fetch request: '%v'", replyEvent.EventType)
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

	data, err := json.Marshal(payment)
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
	render.DefaultResponder(w, r, "Accepted")
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

	rt := models.PaymentResource
	data, err := json.Marshal(models.ResourceLocator{
		ResourceType:   &rt,
		OrganisationID: &oid,
		ID:             &pid,
	})
	if err != nil {
		log.WithError(err).Error("failed to marshal resource resource locator")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg, err := s.Nats.Request(string(models.DeletePaymentEvent), data, s.natsTimeout)
	if err != nil {
		log.WithError(err).Error("fetch request event failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	replyEvent := models.Event{}
	if err := json.Unmarshal(msg.Data, &replyEvent); err != nil {
		logError(w, err, "failed to decode reply data")
		return
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		locator := models.ResourceLocator{}
		if err := json.Unmarshal([]byte(replyEvent.Resource), &locator); err != nil {
			logError(w, err, "failed to decode locator data")
			return
		}
		render.Status(r, http.StatusGone)
		render.DefaultResponder(w, r, locator)
	case models.ResourceNotFoundEvent:
		log.Warnf("resource '%v/%v' was not found", oid.String(), pid.String())
		http.Error(w, fmt.Sprintf("resource '%v/%v' was not found", oid.String(), pid.String()), http.StatusNotFound)
	case models.ServiceErrorEvent:
		if cause, err := jsonparser.GetString([]byte(replyEvent.Resource), "cause"); err != nil {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		} else {
			http.Error(w, cause, http.StatusInternalServerError)
		}
	default:
		logErrorf(w, nil, "unrecognised response to fetch request: '%v'", replyEvent.EventType)
	}
}

func (s *Server) DumpPayment(w http.ResponseWriter, r *http.Request) {
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

	rt := models.PaymentResource
	data, err := json.Marshal(models.ResourceLocator{
		ResourceType:   &rt,
		OrganisationID: &oid,
		ID:             &pid,
	})
	if err != nil {
		log.WithError(err).Error("failed to marshal payment resource locator")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Nats.Publish(string(models.DumpPaymentEvent), data); err != nil {
		log.WithError(err).Error("Failed to publish dump event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, "Accepted")
}

func logError(w http.ResponseWriter, err error, msg string) {
	if err != nil {
		log.WithError(err).Error(msg)
	} else {
		log.Error(msg)
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

func logErrorf(w http.ResponseWriter, err error, msg string, v ...interface{}) {
	if err != nil {
		log.WithError(err).Errorf(msg, v...)
	} else {
		log.Errorf(msg, v...)
	}
	http.Error(w, fmt.Sprintf(msg, v...), http.StatusInternalServerError)
}
