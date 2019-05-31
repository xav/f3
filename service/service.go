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
	"os"
	"os/signal"
	"time"

	"github.com/apex/log"
	"github.com/gomodule/redigo/redis"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/models"
)

type Config struct {
	redisURL      string
	natsURL       string
	natsUserCreds string
	natsKeyFile   string
}

type Service struct {
	ClientID string
	Nats     f3nats.NatsConn
	Redis    redis.Conn
	PreRun   []func(*Service, *Config) error
	PostRun  []func(*Service)
	Handlers map[models.EventType]MsgHandler

	subscriptions map[models.EventType]f3nats.NatsSubscription
	subscribe     func(f3nats.NatsConn, models.EventType, MsgHandler) (f3nats.NatsSubscription, error)
	unsubscribe   func(f3nats.NatsSubscription) error
}

var (
	stopChan       = make(chan os.Signal, 1)
	serviceStarted = make(chan bool)
	serviceStopped = make(chan bool)
	serviceFailed  = make(chan bool)
)

type MsgHandler func(msg *nats.Msg) error

func NewService(clientID string, handlers ...map[models.EventType]MsgHandler) *Service {
	service := &Service{
		ClientID: clientID,
		PreRun: []func(*Service, *Config) error{
			openNatsConnection,
			openRedisConnection,
		},
		PostRun:       make([]func(*Service), 0),
		Handlers:      make(map[models.EventType]MsgHandler, 0),
		subscriptions: make(map[models.EventType]f3nats.NatsSubscription, 0),
		subscribe:     subscribe,
		unsubscribe:   unsubscribe,
	}

	for _, hmap := range handlers {
		for k, v := range hmap {
			service.Handlers[k] = v
		}
	}

	return service
}

func (s *Service) Start(config *Config) error {
	// Pre-runs
	for _, r := range s.PreRun {
		if err := r(s, config); err != nil {
			s.close()
			serviceFailed <- true
			return errors.Wrap(err, "pre-run failed")
		}
	}

	// Subscribe handlers
	for k, v := range s.Handlers {
		sub, err := s.subscribe(s.Nats, k, v)
		if err != nil {
			s.close()
			serviceFailed <- true
			return errors.Wrap(err, "failed to register service subscriptions")
		}
		s.subscriptions[k] = sub
	}

	// Wait for a SIGINT (e.g. triggered by user with CTRL-C)
	// Run cleanup when signal is received
	cleanupDone := make(chan bool)
	signal.Notify(stopChan, os.Interrupt)
	go func() {
		<-stopChan

		log.Info("cleaning up...")
		s.close()

		cleanupDone <- true
	}()

	log.Infof("%v service is running", s.ClientID)
	serviceStarted <- true

	<-cleanupDone
	log.Info("goodbye")
	serviceStopped <- true

	return nil
}

func (s *Service) close() {
	for _, post := range s.PostRun {
		post(s)
	}
}

func openNatsConnection(s *Service, config *Config) error {
	// Connect Options.
	opts := []nats.Option{nats.Name(s.ClientID)}
	opts = setupNatsConnOptions(opts)

	// Use UserCredentials
	if config.natsUserCreds != "" {
		opts = append(opts, nats.UserCredentials(config.natsUserCreds))
	}

	// Use Nkey authentication.
	if config.natsKeyFile != "" {
		opt, err := nats.NkeyOptionFromSeed(config.natsKeyFile)
		if err != nil {
			log.WithError(err).Fatal("failed to load nats seed file")
		}
		opts = append(opts, opt)
	}

	// Connect to NATS
	log.Infof("connecting to nats")
	nc, err := nats.Connect(config.natsURL, opts...)
	if err != nil {
		log.WithError(err).Fatal("failed to connect to NATS. make sure the nats server is running")
	}

	s.Nats = nc

	s.PostRun = append(s.PostRun, closeNatsConnection)
	return nil
}

func closeNatsConnection(s *Service) {
	for _, sub := range s.subscriptions {
		if err := s.unsubscribe(sub); err != nil {
			log.WithError(err).Error("failed to close nats subscription")
		}
	}
	s.Nats.Close()
	log.Info("nats connection closed")
}

func openRedisConnection(s *Service, config *Config) error {
	log.Infof("connecting to redis")
	c, err := redis.DialURL(config.redisURL)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Redis")
	}
	s.Redis = c

	s.PostRun = append(s.PostRun, closeRedisConnection)
	return nil
}

func closeRedisConnection(s *Service) {
	if err := s.Redis.Close(); err != nil {
		log.WithError(err).Error("failed to close redis connection")
	}
	log.Info("redis connection closed")
}

func setupNatsConnOptions(opts []nats.Option) []nats.Option {
	var (
		totalWait      = 5 * time.Minute
		reconnectDelay = time.Second
	)
	opts = append(opts, nats.ReconnectWait(reconnectDelay))
	opts = append(opts, nats.MaxReconnects(int(totalWait/reconnectDelay)))
	opts = append(opts, nats.DisconnectHandler(func(nc *nats.Conn) {
		log.Infof("Disconnected: will attempt reconnects for %.0fm", totalWait.Minutes())
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		log.Infof("Reconnected [%s]", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		log.Fatal("Exiting, no servers available")
	}))
	return opts
}

func subscribe(natsConn f3nats.NatsConn, event models.EventType, handler MsgHandler) (f3nats.NatsSubscription, error) {
	subscription, err := natsConn.Subscribe(string(event), func(msg *nats.Msg) {
		if err := handler(msg); err != nil {
			log.WithError(err).Error("event handler failed")
		}
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to subscribe to '%v'", event)
	}

	return subscription, nil
}

func unsubscribe(subscription f3nats.NatsSubscription) error {
	return subscription.Unsubscribe()
}
