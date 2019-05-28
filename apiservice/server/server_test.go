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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/civil"
	"github.com/apex/log"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/smartystreets/gunit"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/events"
	"gopkg.in/mgo.v2/bson"

	"github.com/xav/f3/apiservice/server/mocks"
)

func TestRoutesFixture(t *testing.T) {
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

func (f *RoutesFixture) TestUpdatePayment() {
	request := httptest.NewRequest("PUT", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("UpdatePayment", f.rr.Body.String())
}

func (f *RoutesFixture) TestFetchPayment() {
	request := httptest.NewRequest("GET", "/v1/orgId/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("FetchPayment", f.rr.Body.String())
}

func (f *RoutesFixture) TestDeletePayment() {
	request := httptest.NewRequest("DELETE", "/v1/orgId/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	f.AssertEqual(http.StatusOK, f.rr.Code)
	f.AssertEqual("DeletePayment", f.rr.Body.String())
}

////////////////////////////////////////

func TestAPICreatePaymentFixture(t *testing.T) {
	gunit.Run(new(APICreatePaymentFixture), t)
}

type APICreatePaymentFixture struct {
	*gunit.Fixture
	server *Server
	nats   *mocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func (f *APICreatePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = f.nats
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}
	f.server = s

	f.rr = httptest.NewRecorder()
}

func (f *APICreatePaymentFixture) TestCreatePayment() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", string(events.CreatePayment), mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusAccepted, f.rr.Code)
}

func (f *APICreatePaymentFixture) TestCreatePayment_BadInput() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "ABCD",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APICreatePaymentFixture) TestCreatePayment_MissingAttributes() {
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb"
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APICreatePaymentFixture) TestCreatePayment_MissingPaymentId() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APICreatePaymentFixture) TestCreatePayment_QueueError() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", string(events.CreatePayment), mock.Anything).
		Return(errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}

////////////////////////////////////////

func TestAPIUpdatePaymentFixture(t *testing.T) {
	gunit.Run(new(APIUpdatePaymentFixture), t)
}

type APIUpdatePaymentFixture struct {
	*gunit.Fixture
	server *Server
	nats   *mocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func (f *APIUpdatePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = f.nats
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}
	f.server = s

	f.rr = httptest.NewRecorder()
}

func (f *APIUpdatePaymentFixture) TestUpdatePayment() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("PUT", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", string(events.UpdatePayment), mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusAccepted, f.rr.Code)
}

func (f *APIUpdatePaymentFixture) TestUpdatePayment_BadInput() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "ABCD",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("PUT", "/v1", strings.NewReader(input))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APIUpdatePaymentFixture) TestUpdatePayment_MissingAttributes() {
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb"
}`
	req := httptest.NewRequest("PUT", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", "payment:create", mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APIUpdatePaymentFixture) TestUpdatePayment_MissingPaymentId() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("PUT", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", "payment:create", mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusBadRequest, f.rr.Code)
}

func (f *APIUpdatePaymentFixture) TestUpdatePayment_QueueError() {
	//noinspection SpellCheckingInspection
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb",
  "attributes": {
    "amount": "100.21",
    "beneficiary_party": {
      "account_name": "W Owens",
      "account_number": "31926819",
      "account_number_code": "BBAN",
      "account_type": 0,
      "address": "1 The Beneficiary Localtown SE2",
      "bank_id": "403000",
      "bank_id_code": "GBDSC",
      "name": "Wilfred Jeremiah Owens"
    },
    "charges_information": {
      "bearer_code": "SHAR",
      "sender_charges": [
        {
          "amount": "5.00",
          "currency": "GBP"
        },
        {
          "amount": "10.00",
          "currency": "USD"
        }
      ],
      "receiver_charges_amount": "1.00",
      "receiver_charges_currency": "USD"
    },
    "currency": "GBP",
    "debtor_party": {
      "account_name": "EJ Brown Black",
      "account_number": "GB29XABC10161234567801",
      "account_number_code": "IBAN",
      "address": "10 Debtor Crescent Sourcetown NE1",
      "bank_id": "203301",
      "bank_id_code": "GBDSC",
      "name": "Emelia Jane Brown"
    },
    "end_to_end_reference": "Wil piano Jan",
    "fx": {
      "contract_reference": "FX123",
      "exchange_rate": "2.00000",
      "original_amount": "200.42",
      "original_currency": "USD"
    },
    "numeric_reference": "1002001",
    "payment_id": "123456789012345678",
    "payment_purpose": "Paying for goods/services",
    "payment_scheme": "FPS",
    "payment_type": "Credit",
    "processing_date": "2017-01-18",
    "reference": "Payment for Em's piano lessons",
    "scheme_payment_sub_type": "InternetBanking",
    "scheme_payment_type": "ImmediatePayment",
    "sponsor_party": {
      "account_number": "56781234",
      "bank_id": "123123",
      "bank_id_code": "GBDSC"
    }
  }
}`
	req := httptest.NewRequest("PUT", "/v1", strings.NewReader(input))

	f.nats.
		On("Publish", string(events.UpdatePayment), mock.Anything).
		Return(errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}

////////////////////////////////////////

func TestAPIFetchPaymentFixture(t *testing.T) {
	gunit.Run(new(APIFetchPaymentFixture), t)
}

type APIFetchPaymentFixture struct {
	*gunit.Fixture
	server *Server
	nats   *mocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func (f *APIFetchPaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = f.nats
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}
	f.server = s

	f.rr = httptest.NewRecorder()
}

func (f *APIFetchPaymentFixture) TestFetchPayment() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	//noinspection SpellCheckingInspection
	data, err := bson.Marshal(Payment{
		Type:           "Payment",
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
		Version:        0,
		Attributes: &PaymentAttributes{
			PaymentID: "123456789012345678",
			Amount:    100.21,
			Currency:  "GBP",
			Purpose:   "Paying for goods/services",
			Scheme:    "FPS",
			Type:      "Credit",
			ProcessingDate: civil.Date{
				Year:  2017,
				Month: 1,
				Day:   18,
			},
			NumericReference:  1002001,
			Reference:         "Payment for Em's piano lessons",
			EndToEndReference: "Wil piano Jan",
			ChargesInformation: ChargesInformation{
				BearerCode: "SHAR",
				SenderCharges: []Charges{
					{
						Amount:   5.00,
						Currency: "GBP",
					}, {
						Amount:   10.00,
						Currency: "USD",
					},
				},
				ReceiverChargesAmount:   1.00,
				ReceiverChargesCurrency: "USD",
			},
			Exchange: Exchange{
				ContractReference: "FX123",
				ExchangeRate:      2.00000,
				OriginalAmount:    200.42,
				OriginalCurrency:  "USD",
			},
			SchemePaymentSubType: "InternetBanking",
			SchemePaymentType:    "ImmediatePayment",
			BeneficiaryParty: Party{
				AccountNumber:     "W Owens",
				BankId:            "403000",
				BankIdCode:        "GBDSC",
				Name:              "Wilfred Jeremiah Owens",
				Address:           "1 The Beneficiary Localtown SE2",
				AccountName:       "W Owens",
				AccountNumberCode: "BBAN",
				AccountType:       0,
			},
			DebtorParty: Party{
				AccountNumber:     "GB29XABC10161234567801",
				BankId:            "203301",
				BankIdCode:        "GBDSC",
				Name:              "Emelia Jane Brown",
				Address:           "10 Debtor Crescent Sourcetown NE1",
				AccountName:       "EJ Brown Black",
				AccountNumberCode: "IBAN",
				AccountType:       0,
			},
			SponsorParty: Party{
				AccountNumber: "56781234",
				BankId:        "123123",
				BankIdCode:    "GBDSC",
			},
		},
	})
	f.Assert(err == nil)

	f.nats.
		On("Request", string(events.FetchPayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: string(events.PaymentFound),
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusOK, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_MissingOrgID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1//%v", locator.ID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_InvalidOrgID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/not-uuid/%v", locator.ID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_InvalidID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/not-uuid", locator.OrganisationID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_QueueError() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.FetchPayment), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_NotFound() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.FetchPayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: string(events.PaymentNotFound),
			Reply:   "",
			Data:    nil,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIFetchPaymentFixture) TestFetchPayment_UnrecognisedResponse() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.FetchPayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "SomethingElse",
			Reply:   "",
			Data:    nil,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}

////////////////////////////////////////

func TestAPIDeletePaymentFixture(t *testing.T) {
	gunit.Run(new(APIDeletePaymentFixture), t)
}

type APIDeletePaymentFixture struct {
	*gunit.Fixture
	server *Server
	nats   *mocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func (f *APIDeletePaymentFixture) Setup() {
	f.nats = &mocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = f.nats
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}
	f.server = s

	f.rr = httptest.NewRecorder()
}

func (f *APIDeletePaymentFixture) TestDeletePayment() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	//noinspection SpellCheckingInspection
	data, err := bson.Marshal(locator)
	f.Assert(err == nil)

	f.nats.
		On("Request", string(events.DeletePayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: string(events.PaymentFound),
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusGone, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_MissingOrgID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1//%v", locator.ID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_InvalidOrgID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/not-uuid/%v", locator.ID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_InvalidID() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/not-uuid", locator.OrganisationID), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_QueueError() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.DeletePayment), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_NotFound() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.DeletePayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: string(events.PaymentNotFound),
			Reply:   "",
			Data:    nil,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusNotFound, f.rr.Code)
}

func (f *APIDeletePaymentFixture) TestDeletePayment_UnrecognisedResponse() {
	locator := ResourceLocator{
		OrganisationID: uuid.New(),
		ID:             uuid.New(),
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	f.nats.
		On("Request", string(events.DeletePayment), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "SomethingElse",
			Reply:   "",
			Data:    nil,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	f.AssertEqual(http.StatusInternalServerError, f.rr.Code)
}
