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
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rafaeljusto/redigomock"
	"github.com/smartystreets/gunit"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/apiservice/server"
	"github.com/xav/f3/events"
	"github.com/xav/f3/events/mocks"
	"gopkg.in/mgo.v2/bson"
)

func TestCreatePaymentFixture(t *testing.T) {
	gunit.Run(new(CreatePaymentFixture), t)
}

type CreatePaymentFixture struct {
	*gunit.Fixture
	nats    *mocks.NatsConn
	redis   *redigomock.Conn
	service *Service
}

func (f *CreatePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	f.redis = redigomock.NewConn()
	f.service = &Service{
		Nats:  f.nats,
		Redis: f.redis,
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := f.service.HandleCreatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_ScanError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := f.service.HandleCreatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_AlreadyPresent() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	if err := f.service.HandleCreatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id already present in store", errors.Cause(err).Error())
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_VersionError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
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

	if err := f.service.HandleCreatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_SetError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
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

	if err := f.service.HandleCreatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_NoReply() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
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

	if err := f.service.HandleCreatePayment(msg); err != nil {
		f.Error(err)
	}
}

func (f *CreatePaymentFixture) TestCreatePayment_Reply() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.CreatePayment),
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

	if err := f.service.HandleCreatePayment(msg); err != nil {
		f.Error(err)
	}
}

////////////////////////////////////////

func TestUpdatePaymentFixture(t *testing.T) {
	gunit.Run(new(UpdatePaymentFixture), t)
}

type UpdatePaymentFixture struct {
	*gunit.Fixture
	nats    *mocks.NatsConn
	redis   *redigomock.Conn
	service *Service
}

func (f *UpdatePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	f.redis = redigomock.NewConn()
	f.service = &Service{
		Nats:  f.nats,
		Redis: f.redis,
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := f.service.HandleUpdatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_ScanError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := f.service.HandleUpdatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_NotPresent() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	if err := f.service.HandleUpdatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id was not found in store", errors.Cause(err).Error())
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_VersionError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	if err := f.service.HandleUpdatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_SetError() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleUpdatePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_NoReply() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleUpdatePayment(msg); err != nil {
		f.Error(err)
	}
}

func (f *UpdatePaymentFixture) TestUpdatePayment_Reply() {
	data, err := bson.Marshal(server.Payment{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.UpdatePayment),
		Reply:   "reply-inbox",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.UpdatePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleUpdatePayment(msg); err != nil {
		f.Error(err)
	}
}

////////////////////////////////////////

func TestDeletePaymentFixture(t *testing.T) {
	gunit.Run(new(DeletePaymentFixture), t)
}

type DeletePaymentFixture struct {
	*gunit.Fixture
	nats    *mocks.NatsConn
	redis   *redigomock.Conn
	service *Service
}

func (f *DeletePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	f.redis = redigomock.NewConn()
	f.service = &Service{
		Nats:  f.nats,
		Redis: f.redis,
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_InvalidPayload() {
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    []byte(("not_bson")),
		Sub:     nil,
	}

	if err := f.service.HandleDeletePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("Document is corrupted", errors.Cause(err).Error())
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_ScanError() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectError(errors.New("redis scan error"))

	if err := f.service.HandleDeletePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis scan error", errors.Cause(err).Error())
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_NotPresent() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{})

	if err := f.service.HandleDeletePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("payment id was not found in store", errors.Cause(err).Error())
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_VersionError() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.DeletePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
	})
	f.Assert(err == nil)
	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{bytes})

	f.redis.
		Command("INCR", redigomock.NewAnyData()).
		ExpectError(errors.New("redis incr error"))

	if err := f.service.HandleDeletePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis incr error", errors.Cause(err).Error())
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_SetError() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.DeletePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleDeletePayment(msg); err == nil {
		f.Error("handler should have failed")
	} else {
		f.AssertEqual("redis set error", errors.Cause(err).Error())
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_NoReply() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.DeletePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleDeletePayment(msg); err != nil {
		f.Error(err)
	}
}

func (f *DeletePaymentFixture) TestDeletePayment_Reply() {
	data, err := bson.Marshal(server.ResourceLocator{})
	f.Assert(err == nil)
	msg := &nats.Msg{
		Subject: string(events.DeletePayment),
		Reply:   "reply-inbox",
		Data:    data,
		Sub:     nil,
	}

	bytes, err := json.Marshal(StoreEvent{
		EventType: events.DeletePayment,
		Version:   int64(1),
		Resource:  &server.Payment{},
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

	if err := f.service.HandleDeletePayment(msg); err != nil {
		f.Error(err)
	}
}
