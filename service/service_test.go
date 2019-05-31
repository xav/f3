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
	"github.com/smartystreets/gunit"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/f3nats/mocks"
	"github.com/xav/f3/models"
)

func TestServiceFixture(t *testing.T) {
	gunit.Run(new(ServiceFixture), t)
}

type ServiceFixture struct {
	*gunit.Fixture
	nats  *mocks.NatsConn
	redis *redigomock.Conn
}

func (f *ServiceFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	f.redis = redigomock.NewConn()
}

func (f *ServiceFixture) fakePreRun(s *Service, config *Config) error {
	s.Nats = f.nats
	s.Redis = f.redis
	return nil
}

func (f *ServiceFixture) fakeSubscribe(natsConn f3nats.NatsConn, event models.EventType, handler MsgHandler) (f3nats.NatsSubscription, error) {
	_, err := subscribe(natsConn, event, handler)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to subscribe to '%v'", event)
	}

	return &mocks.NatsSubscription{}, nil
}

func (f *ServiceFixture) TestNewService_NoHandlers() {
	service := NewService("client id")
	f.Assert(len(service.Handlers) == 0)
}

func (f *ServiceFixture) TestNewService_Handlers() {
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(msg *nats.Msg) error { return nil },
	}
	service := NewService("client id", handlers)
	f.Assert(len(service.Handlers) == 2)
}

func (f *ServiceFixture) TestStartService_NoHandlers() {
	service := NewService("test client")
	service.PreRun = []func(*Service, *Config) error{f.fakePreRun}

	go func() { _ = service.Start(&Config{}) }()
	<-serviceStarted
	f.Assert(len(service.subscriptions) == 0)

	f.nats.On("Close")
	stopChan <- syscall.SIGINT

	<-serviceStopped
}

func (f *ServiceFixture) TestStartService_Handlers() {
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(msg *nats.Msg) error { return nil },
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
	f.Assert(len(service.subscriptions) == 2)

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

func (f *ServiceFixture) TestStartService_Handlers_SubscribeFailed() {
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(msg *nats.Msg) error { return nil },
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

func (f *ServiceFixture) TestStartService_PreRunFailed() {
	handlers := map[models.EventType]MsgHandler{
		models.CreatePaymentEvent: func(msg *nats.Msg) error { return nil },
		models.UpdatePaymentEvent: func(msg *nats.Msg) error { return nil },
	}
	service := NewService("test client", handlers)
	service.PreRun = []func(*Service, *Config) error{func(s *Service, config *Config) error {
		return errors.New("pre-run error")
	}}
	service.subscribe = f.fakeSubscribe

	go func() { _ = service.Start(&Config{}) }()
	<-serviceFailed
}
