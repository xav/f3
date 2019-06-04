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
		scanVersionsKeys: func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID, cursor uint8) (uint8, [][]byte, error) {
			return 0, nil, nil
		},
	}

	if len(s) == 0 {
		return start
	}

	if s[0].scanVersionsKeys != nil {
		start.scanVersionsKeys = s[0].scanVersionsKeys
	}

	return start
}

////////////////////////////////////////

func TestCreatePayment(t *testing.T) {
	t.Run("create payment with invalid payload", TestCreatePayment_InvalidPayload)
	t.Run("create payment where the redis scan fails", TestCreatePayment_ScanError)
	t.Run("create payment where the locator is already present in redis", TestCreatePayment_AlreadyPresent)
	t.Run("create payment where version handler fails", TestCreatePayment_VersionError)
	t.Run("create payment where the redis set fails", TestCreatePayment_SetError)
	t.Run("successful create payment with no reply inbox", TestCreatePayment_NoReply)
	t.Run("successful create payment with reply inbox", TestCreatePayment_Reply)
}

func TestCreatePayment_InvalidPayload(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    []byte(("//")),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestCreatePayment_ScanError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, nil, errors.New("scan error")
		},
	})

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "scan error")
}

func TestCreatePayment_AlreadyPresent(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, err, "payment id already present in store")
}

func TestCreatePayment_VersionError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis incr error")
}

func TestCreatePayment_SetError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis set error")
}

func TestCreatePayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", string(models.PaymentCreatedEvent), mock.Anything).
		Return(nil)

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestCreatePayment_Reply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	err := s.HandleCreatePayment(f.service, &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "reply-inbox",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

////////////////////////////////////////

func TestUpdatePayment(t *testing.T) {
	t.Run("update payment with invalid payload", TestUpdatePayment_InvalidPayload)
	t.Run("update payment where the redis scan fails", TestUpdatePayment_ScanError)
	t.Run("update payment where the locator is not present in redis", TestUpdatePayment_NotPresent)
	t.Run("update payment where version handler fails", TestUpdatePayment_VersionError)
	t.Run("update payment where the redis set fails", TestUpdatePayment_SetError)
	t.Run("successful update payment with no reply inbox", TestUpdatePayment_NoReply)
	t.Run("successful update payment with reply inbox", TestUpdatePayment_Reply)
}

func TestUpdatePayment_InvalidPayload(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    []byte(("//")),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestUpdatePayment_ScanError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, nil, errors.New("scan error")
		},
	})

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "scan error")
}

func TestUpdatePayment_NotPresent(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "payment id was not found in store")
}

func TestUpdatePayment_VersionError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis incr error")
}

func TestUpdatePayment_SetError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis set error")
}

func TestUpdatePayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", string(models.PaymentUpdatedEvent), mock.Anything).
		Return(nil)

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestUpdatePayment_Reply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	err := s.HandleUpdatePayment(f.service, &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "reply-inbox",
		Data:    jsonMarshal(t, models.Payment{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

////////////////////////////////////////

func TestDeletePayment(t *testing.T) {
	t.Run("delete payment with invalid payload", TestDeletePayment_InvalidPayload)
	t.Run("delete payment where the redis scan fails", TestDeletePayment_ScanError)
	t.Run("delete payment where the locator is not present in redis", TestDeletePayment_NotPresent)
	t.Run("delete payment where version handler fails", TestDeletePayment_VersionError)
	t.Run("delete payment where the redis set fails", TestDeletePayment_SetError)
	t.Run("successful delete payment with no reply inbox", TestDeletePayment_NoReply)
	t.Run("successful delete payment with reply inbox", TestDeletePayment_Reply)
}

func TestDeletePayment_InvalidPayload(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    []byte(("//")),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "invalid character '/' looking for beginning of value")
}

func TestDeletePayment_ScanError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, nil, errors.New("scan error")
		},
	})

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "scan error")
}

func TestDeletePayment_NotPresent(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t)

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "payment id was not found in store")
}

func TestDeletePayment_VersionError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis incr error")
}

func TestDeletePayment_SetError(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	require.EqualError(t, errors.Cause(err), "redis set error")
}

func TestDeletePayment_NoReply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", string(models.PaymentDeletedEvent), mock.Anything).
		Return(nil)

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}

func TestDeletePayment_Reply(t *testing.T) {
	f := SetupTest(t)
	s := NewTestStart(t, &Start{
		scanVersionsKeys: func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error) {
			return 0, [][]byte{[]byte("v/Payment/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000")}, nil
		},
	})
	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))
	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")
	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	err := s.HandleDeletePayment(f.service, &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "reply-inbox",
		Data:    jsonMarshal(t, models.ResourceLocator{}),
		Sub:     nil,
	})

	assert.NoError(t, err)
}
