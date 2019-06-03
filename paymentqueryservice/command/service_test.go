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
	"strings"
	"testing"

	"cloud.google.com/go/civil"
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

func NewTestStart(t *testing.T, s ...*Start) *Start {
	t.Helper()

	start := &Start{
		fetchEvents: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID) ([]models.Event, error) {
			return []models.Event{}, nil
		},
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			return []models.ResourceLocator{}, nil
		},
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			return nil, nil
		},
	}

	if len(s) == 0 {
		return start
	}

	if s[0].fetchEvents != nil {
		start.fetchEvents = s[0].fetchEvents
	}
	if s[0].fetchLocators != nil {
		start.fetchLocators = s[0].fetchLocators
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

	_, err := fetchEvents(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	assert.EqualError(t, errors.Cause(err), "redis scan error")
}

func TestFetchEvents_OneBatch(t *testing.T) {
	f := SetupTest(t)

	k := []byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/1")
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{k})
	f.redis.
		Command("GET", k).
		Expect(jsonMarshal(t, models.Event{}))

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestFetchEvents_MultipleBatches(t *testing.T) {
	f := SetupTest(t)
	k := [][]byte{
		[]byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/1"),
		[]byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/2"),
	}
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("1"), []interface{}{k[0]})
	f.redis.
		Command("GET", k[0]).
		Expect(jsonMarshal(t, models.Event{}))
	f.redis.
		Command("SCAN", uint8(1), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{k[1]})
	f.redis.
		Command("GET", k[1]).
		Expect(jsonMarshal(t, models.Event{}))

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestFetchEvents_OutOfOrder(t *testing.T) {
	f := SetupTest(t)
	k := [][]byte{
		[]byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/1"),
		[]byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/2"),
		[]byte("e/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000/3"),
	}
	evt := [][]byte{
		jsonMarshal(t, models.Event{Version: 1}),
		jsonMarshal(t, models.Event{Version: 2}),
		jsonMarshal(t, models.Event{Version: 3}),
	}
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{k[2], k[0], k[1]})
	f.redis.
		Command("GET", k[0]).
		Expect(evt[0])
	f.redis.
		Command("GET", k[1]).
		Expect(evt[1])
	f.redis.
		Command("GET", k[2]).
		Expect(evt[2])

	events, err := fetchEvents(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, int64(1), events[0].Version)
	assert.Equal(t, int64(2), events[1].Version)
	assert.Equal(t, int64(3), events[2].Version)
}

////////////////////////////////////////

func TestFetchLocators(t *testing.T) {
	t.Run("scan error in fetch locators", TestFetchLocators_ScanError)
	t.Run("fetch locators with one batch in scan", TestFetchLocators_OneBatch)
	t.Run("fetch locators with multiple batches in scan", TestFetchLocators_MultipleBatches)
}

func TestFetchLocators_ScanError(t *testing.T) {
	f := SetupTest(t)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	_, err := fetchLocators(f.service.Redis, models.PaymentResource, &uuid.Nil, nil)

	assert.EqualError(t, errors.Cause(err), "redis scan error")
}

func TestFetchLocators_OneBatch(t *testing.T) {
	f := SetupTest(t)

	k := []byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{k})

	events, err := fetchLocators(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestFetchLocators_MultipleBatches(t *testing.T) {
	f := SetupTest(t)
	k := [][]byte{
		[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000"),
		[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000001"),
	}
	f.redis.
		Command("SCAN", uint8(0), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("1"), []interface{}{k[0]})
	f.redis.
		Command("SCAN", uint8(1), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{k[1]})

	events, err := fetchLocators(f.service.Redis, models.PaymentResource, &uuid.Nil, &uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, events, 2)
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
	evt, err := buildPaymentFromEvents([]models.Event{})
	require.NoError(t, err)
	require.NotNil(t, evt)
	require.Equal(t, models.ResourceNotFoundEvent, evt.EventType)
	require.Equal(t, "", evt.Resource)
}

func TestBuildPaymentFromEvents_NilEvents(t *testing.T) {
	evt, err := buildPaymentFromEvents(nil)
	require.NoError(t, err)
	assert.Equal(t, models.ResourceNotFoundEvent, evt.EventType)
	require.Equal(t, "", evt.Resource)
}

func TestBuildPaymentFromEvents_BadSequenceStart(t *testing.T) {
	events := []models.Event{{
		EventType: models.UpdatePaymentEvent,
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid events sequence: sequence should start with a create event")
}

func TestBuildPaymentFromEvents_BadSequenceStartResource(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Resource:  "//",
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid event: the create event resource is not 'Payment'")
}

func TestBuildPaymentFromEvents_NoUpdates(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			OrganisationID: uuid.New(),
			ID:             uuid.New(),
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}}
	evt, err := buildPaymentFromEvents(events)

	require.NoError(t, err)
	assert.Equal(t, models.ResourceFoundEvent, evt.EventType)
	assert.Equal(t, int64(1), evt.Version)
	assert.Equal(t, int64(499137600), evt.CreatedAt)
	require.NotNil(t, evt.UpdatedAt)
	require.Equal(t, int64(499137600), *evt.UpdatedAt)

	require.NotNil(t, evt.Resource)
	p := models.Payment{}
	err = json.Unmarshal([]byte(evt.Resource), &p)
	require.NoError(t, err)
	assert.Equal(t, float32(42), p.Attributes.Amount)
}

func TestBuildPaymentFromEvents_Updates(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}, {
		EventType: models.UpdatePaymentEvent,
		Version:   2,
		CreatedAt: 1445444940,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 2.718,
			},
		})),
	}}
	evt, err := buildPaymentFromEvents(events)

	require.NoError(t, err)
	assert.Equal(t, models.ResourceFoundEvent, evt.EventType)
	assert.Equal(t, int64(2), evt.Version)
	assert.Equal(t, int64(499137600), evt.CreatedAt)
	require.NotNil(t, evt.UpdatedAt)
	require.Equal(t, int64(1445444940), *evt.UpdatedAt)

	require.NotNil(t, evt.Resource)
	p := models.Payment{}
	err = json.Unmarshal([]byte(evt.Resource), &p)
	assert.Equal(t, float32(2.718), p.Attributes.Amount)
}

func TestBuildPaymentFromEvents_UpdatesBadResource(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}, {
		EventType: models.UpdatePaymentEvent,
		Resource:  "//",
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "invalid event: the update event resource is not 'Payment'")
}

func TestBuildPaymentFromEvents_UpdatesBadEventType(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}, {
		EventType: "bad_event",
		Version:   2,
		CreatedAt: 1445444940,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 2.718,
			},
		})),
	}}
	_, err := buildPaymentFromEvents(events)
	assert.EqualError(t, errors.Cause(err), "unrecognised event type: 'bad_event'")
}

func TestBuildPaymentFromEvents_Deleted(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}, {
		EventType: models.DeletePaymentEvent,
		Version:   2,
		CreatedAt: 1445444940,
	}}

	evt, err := buildPaymentFromEvents(events)

	require.NoError(t, err)
	assert.Equal(t, models.ResourceNotFoundEvent, evt.EventType)
	assert.Equal(t, int64(2), evt.Version)
	assert.Equal(t, int64(499137600), evt.CreatedAt)
	require.NotNil(t, evt.UpdatedAt)
	require.Equal(t, int64(1445444940), *evt.UpdatedAt)

	require.Equal(t, "", evt.Resource)
}

func TestBuildPaymentFromEvents_DeletedUpdate(t *testing.T) {
	events := []models.Event{{
		EventType: models.CreatePaymentEvent,
		Version:   1,
		CreatedAt: 499137600,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 42,
			},
		})),
	}, {
		EventType: models.DeletePaymentEvent,
		Version:   2,
		CreatedAt: 872820840,
	}, {
		EventType: models.UpdatePaymentEvent,
		Version:   3,
		CreatedAt: 1445444940,
		Resource: string(jsonMarshal(t, models.Payment{
			Attributes: &models.PaymentAttributes{
				ProcessingDate: civil.Date{
					Year:  2015,
					Month: 10,
					Day:   21,
				},
				Amount: 299792,
			},
		})),
	}}

	evt, err := buildPaymentFromEvents(events)

	require.NoError(t, err)
	assert.Equal(t, models.ResourceNotFoundEvent, evt.EventType)
	assert.Equal(t, int64(2), evt.Version)
	assert.Equal(t, int64(499137600), evt.CreatedAt)
	require.NotNil(t, evt.UpdatedAt)
	require.Equal(t, int64(872820840), *evt.UpdatedAt)

	require.Equal(t, "", evt.Resource)
}

////////////////////////////////////////

func TestHandleFetchPayment(t *testing.T) {
	t.Run("fetch payment with no reply inbox", TestHandleFetchPayment_NoReply)
	t.Run("fetch payment with invalid locator", TestHandleFetchPayment_InvalidLocator)
	t.Run("fetch payment where fetch events fails", TestHandleFetchPayment_FetchEventsError)
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
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})
	assert.EqualError(t, errors.Cause(err), "reply inbox missing from fetch message")
}

func TestHandleFetchPayment_InvalidLocator(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return se.Cause == "failed to unmarshal create event locator"
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    []byte("//"),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestHandleFetchPayment_FetchEventsError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchEvents: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID) ([]models.Event, error) {
			return nil, errors.New("fetch error")
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return strings.HasPrefix(se.Cause, "failed to fetch payment events for ")
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "fetch error")
}

func TestHandleFetchPayment_NoEvents(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			return &models.Event{
				EventType: models.ResourceNotFoundEvent,
			}, nil
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			evt := models.Event{}
			if err := json.Unmarshal(data, &evt); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			return evt.EventType == models.ResourceNotFoundEvent
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleFetchPayment_BuildError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			return nil, errors.New("build error")
		},
	})

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return strings.HasPrefix(se.Cause, "failed to build payment from events for ")
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
	})

	assert.EqualError(t, errors.Cause(err), "build error")
}

func TestHandleFetchPayment_DeletedPayment(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			deletedDate := int64(1445444940)
			return &models.Event{
				EventType: models.ResourceNotFoundEvent,
				Version:   2,
				CreatedAt: 499137600,
				UpdatedAt: &deletedDate,
				Resource:  "",
			}, nil
		},
	})

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			evt := models.Event{}
			if err := json.Unmarshal(data, &evt); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			return evt.EventType == models.ResourceNotFoundEvent
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleFetchPayment_Payment(t *testing.T) {
	f := SetupTest(t)
	resourceID := uuid.New()
	s := NewTestStart(t, &Start{
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			updatedDate := int64(1445444940)
			return &models.Event{
				EventType: models.ResourceFoundEvent,
				Version:   2,
				CreatedAt: 499137600,
				UpdatedAt: &updatedDate,
				Resource: string(jsonMarshal(t, models.Payment{
					ID: resourceID,
				})),
			}, nil
		},
	})

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			evt := models.Event{}
			if err := json.Unmarshal(data, &evt); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			return evt.EventType == models.ResourceFoundEvent
		})).
		Return(nil)

	err := s.HandleFetchPayment(f.service, &nats.Msg{
		Subject: string(models.FetchPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

////////////////////////////////////////

func TestHandleListPayment(t *testing.T) {
	t.Run("fetch payment with no reply inbox", TestHandleListPayment_NoReply)
	t.Run("fetch payment with invalid locator", TestHandleListPayment_InvalidLocator)
	t.Run("fetch payment where fetch locators fails", TestHandleListPayment_FetchLocatorsError)
	t.Run("fetch payment where fetch locators returns nothing", TestHandleListPayment_NoLocators)
	t.Run("fetch payment where fetch events fails", TestHandleListPayment_FetchEventsError)
	t.Run("fetch payment where no events are found or the payment was deleted", TestHandleListPayment_NoEventsOrDeleted)
	t.Run("fetch payment where the payment build fails", TestHandleListPayment_BuildError)
	t.Run("fetch payment on a regular payment", TestHandleListPayment_Payment)
}

func TestHandleListPayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})
	assert.EqualError(t, errors.Cause(err), "reply inbox missing from fetch message")
}

func TestHandleListPayment_InvalidLocator(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return se.Cause == "failed to unmarshal create event locator"
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    []byte(("//")),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestHandleListPayment_FetchLocatorsError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			return nil, errors.New("fetch error")
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return strings.HasPrefix(se.Cause, "failed to fetch payment locators for ")
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "fetch error")
}

func TestHandleListPayment_NoLocators(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			require.Equal(t, models.ResourceFoundEvent, e.EventType)

			assert.Equal(t, "[]", e.Resource)
			return true
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleListPayment_FetchEventsError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			rt := models.PaymentResource
			return []models.ResourceLocator{
				{
					ResourceType:   &rt,
					OrganisationID: &uuid.Nil,
					ID:             &uuid.Nil,
				},
			}, nil
		},
		fetchEvents: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID) ([]models.Event, error) {
			return nil, errors.New("fetch error")
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return strings.HasPrefix(se.Cause, "failed to fetch payment events for ")
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "fetch error")
}

func TestHandleListPayment_NoEventsOrDeleted(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			rt := models.PaymentResource
			return []models.ResourceLocator{
				{
					ResourceType:   &rt,
					OrganisationID: &uuid.Nil,
					ID:             &uuid.Nil,
				},
			}, nil
		},
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			return &models.Event{
				EventType: models.ResourceNotFoundEvent,
			}, nil
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			require.Equal(t, models.ResourceFoundEvent, e.EventType)
			assert.Equal(t, "[]", e.Resource)
			return true
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestHandleListPayment_BuildError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			rt := models.PaymentResource
			return []models.ResourceLocator{
				{
					ResourceType:   &rt,
					OrganisationID: &uuid.Nil,
					ID:             &uuid.Nil,
				},
			}, nil
		},
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			return nil, errors.New("build error")
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			se := models.ServiceError{}
			if err := json.Unmarshal([]byte(e.Resource), &se); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			return strings.HasPrefix(se.Cause, "failed to build payment from events for ")
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.EqualError(t, errors.Cause(err), "build error")
}

func TestHandleListPayment_Payment(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		fetchLocators: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
			rt := models.PaymentResource
			return []models.ResourceLocator{
				{
					ResourceType:   &rt,
					OrganisationID: &uuid.Nil,
					ID:             &uuid.Nil,
				},
			}, nil
		},
		buildPaymentFromEvents: func([]models.Event) (*models.Event, error) {
			updateDate := int64(1445444940)
			return &models.Event{
				EventType: models.ResourceFoundEvent,
				Version:   1,
				CreatedAt: 499137600,
				UpdatedAt: &updateDate,
				Resource:  string(jsonMarshal(t, models.Payment{})),
			}, nil
		},
	})
	f.nats.
		On("Publish", "reply", mock.MatchedBy(func(data []byte) bool {
			e := models.Event{}
			if err := json.Unmarshal(data, &e); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}

			require.Equal(t, models.ResourceFoundEvent, e.EventType)
			p := make([]models.Payment, 0)
			if err := json.Unmarshal([]byte(e.Resource), &p); err != nil {
				t.Error("failed to unmarshal publish data")
				return false
			}
			assert.Len(t, p, 1)
			return true
		})).
		Return(nil)

	err := s.HandleListPayment(f.service, &nats.Msg{
		Subject: string(models.ListPaymentEvent),
		Reply:   "reply",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}
