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

	"github.com/gomodule/redigo/redis"
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
	"gopkg.in/mgo.v2/bson"
)

func jsonMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	j, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func bsonMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
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

func NewTestStart(t *testing.T, s ...*Start) *Start {
	t.Helper()
	start := &Start{
		fetchEvents: func(_ redis.Conn, _ models.ResourceType, _ uuid.UUID, _ uuid.UUID) ([]models.StoreEvent, error) {
			return []models.StoreEvent{}, nil
		},
		buildPaymentFromEvents: func(_ []models.StoreEvent) (*models.Payment, error) {
			return nil, nil
		},
	}

	if len(s) == 0 {
		return start
	}

	if s[0].fetchEvents != nil {
		start.fetchEvents = s[0].fetchEvents
	}
	if s[0].buildPaymentFromEvents != nil {
		start.buildPaymentFromEvents = s[0].buildPaymentFromEvents
	}

	return start
}

////////////////////////////////////////

func TestFetchEvents(t *testing.T) {
	t.Run("scan error in fetch events", TestFetchEvents_ScanError)
	t.Run("fetch events with one batch in scan", TestFetchEvents_OneBatch)
	t.Run("fetch events with multiple batches in scan", TestFetchEvents_MultipleBatches)
	t.Run("fetch events with out of order events from scan", TestFetchEvents_OutOfOrder)
}

func TestFetchEvents_ScanError(t *testing.T) {
	f := SetupTest(t)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	_, err := fetchEvents(f.service.Redis, models.PaymentResource, uuid.Nil, uuid.Nil)

	assert.EqualError(t, errors.Cause(err), "redis scan error")
}

func TestFetchEvents_OneBatch(t *testing.T) {
	f := SetupTest(t)
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{jsonMarshal(t, models.StoreEvent{})})

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, uuid.Nil, uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestFetchEvents_MultipleBatches(t *testing.T) {
	f := SetupTest(t)
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("1"), []interface{}{jsonMarshal(t, models.StoreEvent{})})
	f.redis.
		Command("SCAN", uint8(1), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{jsonMarshal(t, models.StoreEvent{})})

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, uuid.Nil, uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestFetchEvents_OutOfOrder(t *testing.T) {
	f := SetupTest(t)
	evt0 := jsonMarshal(t, models.StoreEvent{Version: 0})
	evt1 := jsonMarshal(t, models.StoreEvent{Version: 1})
	evt2 := jsonMarshal(t, models.StoreEvent{Version: 2})
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{evt2, evt0, evt1})

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, uuid.Nil, uuid.Nil)

	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, int64(0), events[0].Version)
	assert.Equal(t, int64(1), events[1].Version)
	assert.Equal(t, int64(2), events[2].Version)
}

////////////////////////////////////////

func TestBuildPaymentFromEvents(t *testing.T) {
	t.Run("build payment with empty events list", TestBuildPaymentFromEvents_NoEvents)
	t.Run("build payment where events list is nil", TestBuildPaymentFromEvents_NilEvents)
	t.Run("build payment with type other than create for first event in sequence", TestBuildPaymentFromEvents_BadSequenceStart)
	t.Run("build payment with wrong resource type for first event in sequence", TestBuildPaymentFromEvents_BadSequenceStartResource)
	t.Run("build payment with only the initial create event", TestBuildPaymentFromEvents_NoUpdates)
	t.Run("build payment with update events after creation", TestBuildPaymentFromEvents_Updates)
	t.Run("build payment with an invalid event type update events", TestBuildPaymentFromEvents_UpdatesBadEventType)
	t.Run("build payment with wrong resource type for the update events", TestBuildPaymentFromEvents_UpdatesBadResource)
	t.Run("build payment on a deleted payment", TestBuildPaymentFromEvents_Deleted)
	t.Run("build payment with an update event coming in after a delete event", TestBuildPaymentFromEvents_DeletedUpdate)
}

func TestBuildPaymentFromEvents_NoEvents(t *testing.T) {
	p, err := buildPaymentFromEvents([]models.StoreEvent{})
	assert.NoError(t, err)
	assert.Nil(t, p)
}

func TestBuildPaymentFromEvents_NilEvents(t *testing.T) {
	p, err := buildPaymentFromEvents(nil)
	assert.NoError(t, err)
	assert.Nil(t, p)
}

func TestBuildPaymentFromEvents_BadSequenceStart(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.UpdatePaymentEvent,
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid events sequence: sequence should start with a create event")
}

func TestBuildPaymentFromEvents_BadSequenceStartResource(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource:  struct{}{},
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid event: the create event resource is not 'Payment'")
}

func TestBuildPaymentFromEvents_NoUpdates(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}}
	p, err := buildPaymentFromEvents(events)
	assert.NoError(t, err)
	assert.NotNil(t, p.Attributes)
	assert.Equal(t, float32(42), p.Attributes.Amount)
}

func TestBuildPaymentFromEvents_Updates(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}, {
		EventType: models.UpdatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 2.718,
			},
		},
	}}
	p, err := buildPaymentFromEvents(events)
	assert.NoError(t, err)
	assert.NotNil(t, p.Attributes)
	assert.Equal(t, float32(2.718), p.Attributes.Amount)
}

func TestBuildPaymentFromEvents_UpdatesBadResource(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}, {
		EventType: models.UpdatePaymentEvent,
		Resource:  &struct{}{},
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid event: the update event resource is not 'Payment'")
}

func TestBuildPaymentFromEvents_UpdatesBadEventType(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}, {
		EventType: "bad_event",
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 6.626,
			},
		},
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "unrecognised event type: 'bad_event'")
}

func TestBuildPaymentFromEvents_Deleted(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}, {
		EventType: models.DeletePaymentEvent,
	}}
	p, err := buildPaymentFromEvents(events)
	assert.NoError(t, err)
	assert.Nil(t, p)
}

func TestBuildPaymentFromEvents_DeletedUpdate(t *testing.T) {
	events := []models.StoreEvent{{
		EventType: models.CreatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 42,
			},
		},
	}, {
		EventType: models.DeletePaymentEvent,
	}, {
		EventType: models.UpdatePaymentEvent,
		Resource: &models.Payment{
			Attributes: &models.PaymentAttributes{
				Amount: 299792,
			},
		},
	}}
	p, err := buildPaymentFromEvents(events)
	assert.NoError(t, err)
	assert.Nil(t, p)
}

////////////////////////////////////////

func TestHandleFetchPayment(t *testing.T) {
	t.Run("fetch payment with no reply inbox", TestHandleFetchPayment_NoReply)
	t.Run("fetch payment with invalid locator bson", TestHandleFetchPayment_InvalidLocator)
	t.Run("fetch payment where fetch events fails", TestHandleFetchPayment_FetchError)
	t.Run("fetch payment where no events are found", TestHandleFetchPayment_NoEvents)
	t.Run("fetch payment where the payment build fails", TestHandleFetchPayment_BuildError)
	t.Run("fetch payment on a deleted payment", TestHandleFetchPayment_DeletedPayment)
	t.Run("fetch payment on a regular payment", TestHandleFetchPayment_Payment)
}

func TestHandleFetchPayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})
	assert.EqualError(t, errors.Cause(err), "reply inbox missing from fetch message")
}

func TestHandleFetchPayment_InvalidLocator(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Publish", "reply", []byte("failed to unmarshal create event locator")).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	})
	assert.EqualError(t, errors.Cause(err), "Document is corrupted")
}

func TestHandleFetchPayment_FetchError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchEvents: func(_ redis.Conn, _ models.ResourceType, _ uuid.UUID, _ uuid.UUID) ([]models.StoreEvent, error) {
			return nil, errors.New("fetch error")
		},
	})
	f.nats.
		On("Publish", "reply", []byte("failed to fetch payment events for '00000000-0000-0000-0000-000000000000 / 00000000-0000-0000-0000-000000000000'")).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "fetch error")
}

func TestHandleFetchPayment_NoEvents(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.nats.
		On("Publish", "reply", []byte(nil)).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleFetchPayment_BuildError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchEvents: func(_ redis.Conn, _ models.ResourceType, _ uuid.UUID, _ uuid.UUID) ([]models.StoreEvent, error) {
			return []models.StoreEvent{
				{
					EventType: models.CreatePaymentEvent,
					Version:   0,
					CreatedAt: 0,
					Resource:  &models.Payment{},
				},
			}, nil
		},
		buildPaymentFromEvents: func(events []models.StoreEvent) (*models.Payment, error) {
			return nil, errors.New("build error")
		},
	})

	f.nats.
		On("Publish", "reply", []byte("failed to build payment from events for '00000000-0000-0000-0000-000000000000 / 00000000-0000-0000-0000-000000000000'")).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "build error")
}

func TestHandleFetchPayment_DeletedPayment(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchEvents: func(_ redis.Conn, _ models.ResourceType, _ uuid.UUID, _ uuid.UUID) ([]models.StoreEvent, error) {
			return []models.StoreEvent{
				{
					EventType: models.CreatePaymentEvent,
					Version:   0,
					CreatedAt: 0,
					Resource:  &models.Payment{},
				}, {
					EventType: models.DeletePaymentEvent,
				},
			}, nil
		},
		buildPaymentFromEvents: func(events []models.StoreEvent) (*models.Payment, error) {
			return nil, nil
		},
	})

	f.nats.
		On("Publish", "reply", []byte(nil)).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleFetchPayment_Payment(t *testing.T) {
	f := SetupTest(t)
	resourceID := uuid.New()
	s := NewTestStart(t, &Start{
		fetchEvents: func(_ redis.Conn, _ models.ResourceType, _ uuid.UUID, _ uuid.UUID) ([]models.StoreEvent, error) {
			return []models.StoreEvent{
				{
					EventType: models.CreatePaymentEvent,
					Version:   0,
					CreatedAt: 0,
					Resource: &models.Payment{
						ID: resourceID,
					},
				},
			}, nil
		},
		buildPaymentFromEvents: func(events []models.StoreEvent) (*models.Payment, error) {
			return &models.Payment{
				ID: resourceID,
			}, nil
		},
	})

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			p := models.Payment{}
			if err := bson.Unmarshal(data, &p); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return p.ID == resourceID
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    bsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}
