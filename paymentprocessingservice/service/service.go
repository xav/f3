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

package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/apex/log"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/xav/f3/apiservice/server"
	"github.com/xav/f3/events"
	"gopkg.in/mgo.v2/bson"
)

type Service struct {
	ClientID      string
	Nats          events.NatsConn
	Redis         redis.Conn
	subscriptions []*nats.Subscription
	PreRun        []func(*Service) error
}

type StoreEvent struct {
	events.EventType
	Version  int64
	Resource interface{}
}

const paymentResource = "payment"

const (
	versionKeyTemplate   = "v/%v/%v/%v"
	eventKeyTemplate     = "e/%v/%v/%v/%v"
	eventKeyScanTemplate = "e/%v/%v/%v/*"
)

type MsgHandler func(msg *nats.Msg) error

func (s *Service) Start() error {
	for _, r := range s.PreRun {
		if err := r(s); err != nil {
			log.WithError(err).Fatal("initialisation error")
		}
	}

	s.subscriptions = make([]*nats.Subscription, 0, 3)
	s.subscribe(events.CreatePayment, s.HandleCreatePayment)
	s.subscribe(events.UpdatePayment, s.HandleUpdatePayment)
	s.subscribe(events.DeletePayment, s.HandleDeletePayment)

	// Wait for a SIGINT (e.g. triggered by user with CTRL-C)
	// Run cleanup when signal is received
	stopChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(stopChan, os.Interrupt)
	go func() {
		for range stopChan {
			log.Info("unsubscribing and closing connections...")

			// NATS
			for _, s := range s.subscriptions {
				_ = s.Unsubscribe()
			}
			s.Nats.Close()

			// Redis
			if err := s.Redis.Close(); err != nil {
				log.WithError(err).Error("failed to close redis connection")
			}

			cleanupDone <- true
		}
	}()

	log.Info("f3 payment processing service is running")

	<-cleanupDone
	log.Info("goodbye")

	return nil
}

func (s *Service) HandleCreatePayment(msg *nats.Msg) error {
	// Decode the event
	payment := server.Payment{}
	if err := bson.Unmarshal(msg.Data, &payment); err != nil {
		return errors.Wrap(err, "failed to unmarshal create payment event")
	}

	// Check if the payment is already present
	_, evts, err := s.scanResources(paymentResource, payment.OrganisationID, payment.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check existing payments")
	}
	if len(evts) > 0 {
		return errors.New("payment id already present in store")
	}

	// Create the version index
	versionKey := fmt.Sprintf(versionKeyTemplate, paymentResource, payment.OrganisationID, payment.ID)
	version, err := s.Redis.Do("INCR", versionKey)
	if err != nil {
		return errors.Wrap(err, "failed to increment aggregate version")
	}

	// Save the event to the store
	bytes, err := json.Marshal(StoreEvent{
		EventType: events.CreatePayment,
		Version:   version.(int64),
		Resource:  &payment,
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal event data")
	}
	eventKey := fmt.Sprintf(eventKeyTemplate, paymentResource, payment.OrganisationID, payment.ID, version)
	reply, err := s.Redis.Do("SET", eventKey, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to store payment event")
	}

	// Reply if needed
	if msg.Reply != "" {
		if err = s.Nats.Publish(msg.Reply, []byte(reply.(string))); err != nil {
			return errors.Wrap(err, "failed to reply to request")
		}
	}

	log.Infof("created payment '%v / %v'", payment.OrganisationID, payment.ID)
	return nil
}

func (s *Service) HandleUpdatePayment(msg *nats.Msg) error {
	// Decode the event
	payment := server.Payment{}
	if err := bson.Unmarshal(msg.Data, &payment); err != nil {
		return errors.Wrap(err, "failed to unmarshal update payment event")
	}

	// Check if the payment is already present
	_, evts, err := s.scanResources(paymentResource, payment.OrganisationID, payment.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check existing payments")
	}
	if len(evts) == 0 {
		return errors.New("payment id was not found in store")
	}

	// Increment the version index
	versionKey := fmt.Sprintf(versionKeyTemplate, paymentResource, payment.OrganisationID, payment.ID)
	version, err := s.Redis.Do("INCR", versionKey)
	if err != nil {
		return errors.Wrap(err, "failed to increment aggregate version")
	}

	// Save the event to the store
	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   version.(int64),
		Resource:  &payment,
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal event data")
	}
	eventKey := fmt.Sprintf(eventKeyTemplate, paymentResource, payment.OrganisationID, payment.ID, version)
	reply, err := s.Redis.Do("SET", eventKey, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to store payment event")
	}

	// Reply if needed
	if msg.Reply != "" {
		if err = s.Nats.Publish(msg.Reply, []byte(reply.(string))); err != nil {
			return errors.Wrap(err, "failed to reply to request")
		}
	}

	log.Infof("update payment '%v / %v' (%v)", payment.OrganisationID, payment.ID, version)
	return nil
}

func (s *Service) HandleDeletePayment(msg *nats.Msg) error {
	log.Infof("delete payment")
	return nil
}

func (s *Service) scanResources(resourceType string, organizationID uuid.UUID, resourceID uuid.UUID, cursor uint8) (uint8, [][]byte, error) {
	var (
		items   [][]byte
		scanKey = fmt.Sprintf(eventKeyScanTemplate, resourceType, organizationID, resourceID)
	)

	existing, err := redis.Values(s.Redis.Do("SCAN", cursor, "MATCH", scanKey))
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to scan '%v' resources '%v/%v'", resourceType, organizationID, resourceID)
	}
	if _, err := redis.Scan(existing, &cursor, &items); err != nil {
		return 0, nil, errors.Wrap(err, "failed to parse resources list")
	}

	return cursor, items, nil
}

func (s *Service) subscribe(event events.EventType, handler MsgHandler) {
	subscription, err := s.Nats.Subscribe(string(event), func(msg *nats.Msg) {
		if err := handler(msg); err != nil {
			log.WithError(err).Error("event handler failed")
		}
	})
	if err != nil {
		for _, s := range s.subscriptions {
			_ = s.Unsubscribe()
		}
		s.Nats.Close()
		log.Fatalf("failed to subscribe to '%v'", event)
	}

	s.subscriptions = append(s.subscriptions, subscription)
}
