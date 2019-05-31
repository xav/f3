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
	"testing"

	"github.com/rafaeljusto/redigomock"
	"github.com/smartystreets/gunit"
	"github.com/xav/f3/f3nats/mocks"
	"github.com/xav/f3/service"
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
