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

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rafaeljusto/redigomock"
	"github.com/smartystreets/gunit"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/f3nats/mocks"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
	"gopkg.in/mgo.v2/bson"
)

func TestPaymentServiceFixture(t *testing.T) {
	gunit.Run(new(PaymentServiceFixture), t)
}

type PaymentServiceFixture struct {
	*gunit.Fixture
	nats    *mocks.NatsConn
	redis   *redigomock.Conn
	service *service.Service
}

func (f *PaymentServiceFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	f.redis = redigomock.NewConn()
	f.service = service.NewService("test client")
	f.service.Nats = f.nats
	f.service.Redis = f.redis
}

////////////////////////////////////////

func (f *PaymentServiceFixture) TestCreatePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := HandleCreatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_ScanError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := HandleCreatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_AlreadyPresent() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.UpdatePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	if err := HandleCreatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id already present in store", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_VersionError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	if err := HandleCreatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_SetError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	if err := HandleCreatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_NoReply() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	if err := HandleCreatePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}

func (f *PaymentServiceFixture) TestCreatePayment_Reply() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.CreatePaymentEvent),
		Reply:   "reply-inbox",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	if err := HandleCreatePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}

////////////////////////////////////////

func (f *PaymentServiceFixture) TestUpdatePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := HandleUpdatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_ScanError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := HandleUpdatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_NotPresent() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	if err := HandleUpdatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id was not found in store", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_VersionError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.UpdatePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	if err := HandleUpdatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_SetError() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.UpdatePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	if err := HandleUpdatePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_NoReply() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.UpdatePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	if err := HandleUpdatePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}

func (f *PaymentServiceFixture) TestUpdatePayment_Reply() {
	data, err := bson.Marshal(models.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.UpdatePaymentEvent),
		Reply:   "reply-inbox",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.UpdatePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	if err := HandleUpdatePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}

////////////////////////////////////////

func (f *PaymentServiceFixture) TestDeletePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := HandleDeletePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_ScanError() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := HandleDeletePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_NotPresent() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	if err := HandleDeletePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id was not found in store", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_VersionError() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.DeletePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	if err := HandleDeletePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_SetError() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.DeletePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis set error"))

	if err := HandleDeletePayment(f.service, msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_NoReply() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.DeletePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	if err := HandleDeletePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}

func (f *PaymentServiceFixture) TestDeletePayment_Reply() {
	data, err := bson.Marshal(models.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(models.DeletePaymentEvent),
		Reply:   "reply-inbox",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(models.StoreEvent{
		EventType: models.DeletePaymentEvent,
		Version:   int64(1),
		Resource:  &models.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		Expect(int64(1))

	f.redis.
		Command("SET", redigomock.NewAnyData(), redigomock.NewAnyData()).
		Expect("OK")

	f.nats.
		On("Publish", "reply-inbox", mock.Anything).
		Return(nil)

	if err := HandleDeletePayment(f.service, msg); err != nil {
		f.Error(err)
	}
}
