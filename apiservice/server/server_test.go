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

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apex/log"
	"github.com/smartystreets/gunit"
	"github.com/stretchr/testify/mock"

	"github.com/xav/f3/apiservice/server/mocks"
)

func TestServerFixture(t *testing.T) {
	gunit.Run(new(RoutesFixture), t)
}

type RoutesFixture struct {
	*gunit.Fixture
	server *Server
	rr     *httptest.ResponseRecorder
}

func (f *RoutesFixture) Setup() {
	routesHandler := &mocks.RoutesHandler{}
	registerCall := func(name string) {
		routesHandler.
			On(name, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				w := args.Get(0).(http.ResponseWriter)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(name))
			})
	}
	registerCall("ListVersions")
	registerCall("ListPayments")
	registerCall("CreatePayment")
	registerCall("FetchPayment")
	registerCall("UpdatePayment")
	registerCall("DeletePayment")

	s, err := NewServer(func(s *Server) error {
		s.Nats = &mocks.NatsConn{}
		s.routesHandler = routesHandler
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}
	f.server = s

	f.rr = httptest.NewRecorder()
}

func (f *RoutesFixture) TestListVersions() {
	request := httptest.NewRequest("GET", "/", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("ListVersions", f.rr.Body.String())
}

func (f *RoutesFixture) TestListVersions_WrongMethod() {
	request := httptest.NewRequest("POST", "/", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusMethodNotAllowed, f.rr.Code)
}

func (f *RoutesFixture) TestListPayments() {
	request := httptest.NewRequest("GET", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("ListPayments", f.rr.Body.String())
}

func (f *RoutesFixture) TestCreatePayment() {
	request := httptest.NewRequest("POST", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("CreatePayment", f.rr.Body.String())
}

func (f *RoutesFixture) TestFetchPayment() {
	request := httptest.NewRequest("GET", "/v1/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("FetchPayment", f.rr.Body.String())
}

func (f *RoutesFixture) TestUpdatePayment() {
	request := httptest.NewRequest("PUT", "/v1/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("UpdatePayment", f.rr.Body.String())
}

func (f *RoutesFixture) TestDeletePayment() {
	request := httptest.NewRequest("DELETE", "/v1/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("DeletePayment", f.rr.Body.String())
}