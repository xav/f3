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

	f.redis.
		Command("SCAN", redigomock.NewAnyData(), redigomock.NewAnyData(), redigomock.NewAnyData()).
		ExpectSlice([]byte("0"), []interface{}{[]byte("val")})

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
