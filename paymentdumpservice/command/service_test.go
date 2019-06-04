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

package command

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xav/f3/f3nats/mocks"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
)

func jsonMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	j, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

type PaymentServiceTestFixture struct {
	nats    *mocks.NatsConn
	redis   *redigomock.Conn
	service *service.Service
}

func SetupTest(t *testing.T) *PaymentServiceTestFixture {
	t.Helper()
	f := &PaymentServiceTestFixture{
		nats:    &mocks.NatsConn{},
		redis:   redigomock.NewConn(),
		service: service.NewService("test client"),
	}
	f.service.Nats = f.nats
	f.service.Redis = f.redis
	return f
}

func NewTestStart(t *testing.T) *Start {
	t.Helper()
	return NewStart()
}

////////////////////////////////////////

func TestHandleDumpPayment(t *testing.T) {
	t.Run("dump payment with invalid payload", TestHandleDumpPayment_InvalidPayload)
	t.Run("dump payment where fetch nats request fails", TestHandleDumpPayment_QueueError)
	t.Run("dump payment where query service failed to process the fetch request", TestHandleDumpPayment_ServiceError)
	t.Run("dump payment where the event type of the reply is not recognised", TestHandleDumpPayment_UnrecognisedResponse)
	t.Run("dump payment where payment is not found", TestHandleDumpPayment_NotFound)
	t.Run("dump payment where the redis set fails", TestHandleDumpPayment_SetError)
	t.Run("successful dump payment with no reply inbox", TestHandleDumpPayment_NoReply)
	t.Run("successful dump payment with no reply inbox", TestHandleDumpPayment_Reply)
}

func TestHandleDumpPayment_InvalidPayload(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    []byte(("//")),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestHandleDumpPayment_QueueError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "queue error")
}

func TestHandleDumpPayment_UnrecognisedResponse(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	locator := models.ResourceLocator{
		OrganisationID: &uuid.Nil,
		ID:             &uuid.Nil,
	}
	updateDate := int64(1445444940)
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data: jsonMarshal(t, models.Event{
				EventType: "jaqen h'ghar",
				Version:   2,
				CreatedAt: 499137600,
				UpdatedAt: &updateDate,
				Resource:  string(jsonMarshal(t, locator)),
			}),
			Sub: nil,
		}, nil)

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "unrecognized event type")
}

func TestHandleDumpPayment_NotFound(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	id := uuid.New()
	locator := models.ResourceLocator{
		OrganisationID: &id,
		ID:             &id,
	}
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "",
			Reply:   "",
			Data: jsonMarshal(t, models.Event{
				EventType: models.ResourceNotFoundEvent,
				Resource:  string(jsonMarshal(t, locator)),
			}),
			Sub: nil,
		}, nil)

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, locator),
		Sub:     nil,
	})

	require.NoError(t, err)
}

func TestHandleDumpPayment_SetError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "",
			Reply:   "",
			Data: jsonMarshal(t, models.Event{
				EventType: models.ResourceFoundEvent,
				Resource:  string(jsonMarshal(t, models.Payment{})),
			}),
			Sub: nil,
		}, nil)
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis set error")
}

func TestHandleDumpPayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "",
			Reply:   "",
			Data: jsonMarshal(t, models.Event{
				EventType: models.ResourceFoundEvent,
				Resource:  string(jsonMarshal(t, models.Payment{})),
			}),
			Sub: nil,
		}, nil)
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.NoError(t, err)
}

func TestHandleDumpPayment_Reply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "",
			Reply:   "reply",
			Data: jsonMarshal(t, models.Event{
				EventType: models.ResourceFoundEvent,
				Resource:  string(jsonMarshal(t, models.Payment{})),
			}),
			Sub: nil,
		}, nil)
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", "reply", "OK").
		Return(nil)

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.NoError(t, err)
}

func TestHandleDumpPayment_ServiceError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data: jsonMarshal(t, models.Event{
				EventType: models.ServiceErrorEvent,
				Resource: string(jsonMarshal(t, models.ServiceError{
					Cause:   "service failed",
					Request: nil,
				})),
			}),
			Sub: nil,
		}, nil)

	err := s.HandleDumpPayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "service error")
}
