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
	"syscall"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/f3nats/mocks"
	"github.com/xav/f3/models"
)

type ServiceTestFixture struct {
	nats  *mocks.NatsConn
	redis *redigomock.Conn
}

func SetupTest(t *testing.T) *ServiceTestFixture {
	t.Helper()
	return &ServiceTestFixture{
		nats:  &mocks.NatsConn{},
		redis: redigomock.NewConn(),
	}
}

func (f *ServiceTestFixture) fakePreRun(s *Service, config *Config) error {
	s.Nats = f.nats
	s.Redis = f.redis
	return nil
}

func (f *ServiceTestFixture) fakeSubscribe(s *Service, event models.EventType, handler MsgHandler) (f3nats.NatsSubscription, error) {
	_, err := subscribe(s, event, handler)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to subscribe to '%v'", event)
	}

	return &mocks.NatsSubscription{}, nil
}

func TestNewService_NoHandlers(t *testing.T) {
	service := NewService("client id")
	assert.Len(t, service.Handlers, 0)
}

func TestNewService_Handlers(t *testing.T) {
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
	}
	service := NewService("client id", handlers)
	assert.Len(t, service.Handlers, 2)
}

func TestStartService_NoHandlers(t *testing.T) {
	f := SetupTest(t)
	service := NewService("test client")
	service.PreRun = []func(*Service, *Config) error{f.fakePreRun}

	go func() { _ = service.Start(&Config{}) }()
	<-serviceStarted
	assert.Len(t, service.subscriptions, 0)

	f.nats.On("Close")
	stopChan <- syscall.SIGINT

	<-serviceStopped
}

func TestStartService_Handlers(t *testing.T) {
	f := SetupTest(t)
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
	}
	service := NewService("test client", handlers)
	service.PreRun = []func(*Service, *Config) error{f.fakePreRun}
	service.subscribe = f.fakeSubscribe

	f.nats.
		On("Subscribe", mock.Anything, mock.Anything).
		Return(&nats.Subscription{}, nil).
		Times(2)

	go func() { _ = service.Start(&Config{}) }()
	<-serviceStarted
	assert.Len(t, service.subscriptions, 2)

	for _, v := range service.subscriptions {
		sub := v.(*mocks.NatsSubscription)
		sub.
			On("Unsubscribe").
			Return(nil).
			Once()
	}

	f.nats.On("Close").Once()
	stopChan <- syscall.SIGINT

	<-serviceStopped
}

func TestStartService_Handlers_SubscribeFailed(t *testing.T) {
	f := SetupTest(t)
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
	}
	service := NewService("test client", handlers)
	service.PreRun = []func(*Service, *Config) error{f.fakePreRun}
	service.subscribe = f.fakeSubscribe

	f.nats.
		On("Subscribe", mock.Anything, mock.Anything).
		Return(nil, errors.New("nats error")).
		Once()

	f.nats.On("Close").Once()

	go func() { _ = service.Start(&Config{}) }()
	<-serviceFailed
}

func TestStartService_PreRunFailed(t *testing.T) {
	f := SetupTest(t)
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(s *Service, msg *nats.Msg) error { return nil },
	}
	service := NewService("test client", handlers)
	service.PreRun = []func(*Service, *Config) error{func(s *Service, config *Config) error {
		return errors.New("pre-run error")
	}}
	service.subscribe = f.fakeSubscribe

	go func() { _ = service.Start(&Config{}) }()
	<-serviceFailed
}
