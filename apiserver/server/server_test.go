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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xav/f3/models"
	"gopkg.in/mgo.v2/bson"

	natsmocks "github.com/xav/f3/f3nats/mocks"

	"github.com/xav/f3/apiserver/server/mocks"
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

type RoutesTestFixture struct {
	server *Server
	nats   *natsmocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func SetupRoutesTest(t *testing.T) *RoutesTestFixture {
	t.Helper()

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

	nc := &natsmocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = nc
		s.routesHandler = routesHandler
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}

	return &RoutesTestFixture{
		server: s,
		nats:   nc,
		rr:     httptest.NewRecorder(),
	}
}

func TestRoutes(t *testing.T) {
	t.Run("check that 'GET /' is routed to 'ListVersions'", TestRoutes_ListVersions)
	t.Run("check that 'POST /' is not routed to anything", TestRoutes_ListVersions_WrongMethod)
	t.Run("check that 'GET /v1' is routed to 'ListPayments'", TestRoutes_ListPayments)
	t.Run("check that 'POST /v1' is routed to 'CreatePayment'", TestRoutes_CreatePayment)
	t.Run("check that 'PUT /v1' is routed to 'UpdatePayment'", TestRoutes_UpdatePayment)
	t.Run("check that 'GET /v1/{orgId}/{paymentId}' is routed to 'FetchPayment'", TestRoutes_FetchPayment)
	t.Run("check that 'DELETE /v1/{orgId}/{paymentId}' is routed to 'DeletePayment'", TestRoutes_DeletePayment)
}

func TestRoutes_ListVersions(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("GET", "/", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "ListVersions", f.rr.Body.String())
}

func TestRoutes_ListVersions_WrongMethod(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("POST", "/", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusMethodNotAllowed, f.rr.Code)
}

func TestRoutes_ListPayments(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("GET", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "ListPayments", f.rr.Body.String())
}

func TestRoutes_CreatePayment(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("POST", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "CreatePayment", f.rr.Body.String())
}

func TestRoutes_UpdatePayment(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("PUT", "/v1", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "UpdatePayment", f.rr.Body.String())
}

func TestRoutes_FetchPayment(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("GET", "/v1/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "FetchPayment", f.rr.Body.String())
}

func TestRoutes_DeletePayment(t *testing.T) {
	f := SetupRoutesTest(t)
	request := httptest.NewRequest("DELETE", "/v1/orgId/paymentId", nil)
	f.server.Router.ServeHTTP(f.rr, request)
	assert.Equal(t, http.StatusOK, f.rr.Code)
	assert.Equal(t, "DeletePayment", f.rr.Body.String())
}

////////////////////////////////////////

type APIServerTestFixture struct {
	server *Server
	nats   *natsmocks.NatsConn
	rr     *httptest.ResponseRecorder
}

func SetupAPIServerTest(t *testing.T) *APIServerTestFixture {
	t.Helper()

	nc := &natsmocks.NatsConn{}
	s, err := NewServer(func(s *Server) error {
		s.Nats = nc
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}

	return &APIServerTestFixture{
		server: s,
		nats:   nc,
		rr:     httptest.NewRecorder(),
	}
}

////////////////////////////////////////

func TestListPayment(t *testing.T) {
	t.Run("list payments is successful", TestListPayment_Success)
	t.Run("query service failed to process the list request", TestListPayment_ServiceError)
	t.Run("list payment where nats request fails", TestListPayment_QueueError)
	t.Run("list payment where the event type of the reply is not recognised", TestListPayment_UnrecognisedResponse)
}

func TestListPayment_Success(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", "/v1", nil)

	data := bsonMarshal(t, models.Event{
		EventType: models.ResourceFoundEvent,
		Resource:  []*models.Payment{{}, {}},
	})
	f.nats.
		On("Request", string(models.ListPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusOK, f.rr.Code)
}

func TestListPayment_ServiceError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", "/v1", nil)

	data := bsonMarshal(t, models.Event{
		EventType: models.ServiceErrorEvent,
		Resource: models.ServiceError{
			Cause:   "service error",
			Request: nil,
		},
	})
	f.nats.
		On("Request", string(models.ListPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "service error\n", f.rr.Body.String())
}

func TestListPayment_QueueError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", "/v1", nil)

	f.nats.
		On("Request", string(models.ListPaymentEvent), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
}

func TestListPayment_UnrecognisedResponse(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", "/v1", nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: "waif",
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &models.Payment{},
	})
	f.nats.
		On("Request", string(models.ListPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "unrecognised response to fetch request: 'waif'\n", f.rr.Body.String())
}

////////////////////////////////////////

func TestCreatePayment(t *testing.T) {
	t.Run("create payment is handled successfully", TestCreatePayment_Success)
	t.Run("create payment with invalid json input", TestCreatePayment_BadInput)
	t.Run("create payment with required attributes missing from input", TestCreatePayment_MissingAttributes)
	t.Run("create payment with payment id missing from input", TestCreatePayment_MissingPaymentId)
	t.Run("create payment where nats publish fails", TestCreatePayment_QueueError)
}

func TestCreatePayment_Success(t *testing.T) {
	f := SetupAPIServerTest(t)
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
		On("Publish", string(models.CreatePaymentEvent), mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusAccepted, f.rr.Code)
}

func TestCreatePayment_BadInput(t *testing.T) {
	f := SetupAPIServerTest(t)
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
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestCreatePayment_MissingAttributes(t *testing.T) {
	f := SetupAPIServerTest(t)
	input := `{
  "type": "Payment",
  "id": "4ee3a8d8-ca7b-4290-a52c-dd5b6165ec43",
  "version": 0,
  "organisation_id": "743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb"
}`
	req := httptest.NewRequest("POST", "/v1", strings.NewReader(input))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestCreatePayment_MissingPaymentId(t *testing.T) {
	f := SetupAPIServerTest(t)
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
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestCreatePayment_QueueError(t *testing.T) {
	f := SetupAPIServerTest(t)
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
		On("Publish", string(models.CreatePaymentEvent), mock.Anything).
		Return(errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
}

////////////////////////////////////////

func TestUpdatePayment(t *testing.T) {
	t.Run("update payment is handled successfully", TestUpdatePayment_Success)
	t.Run("update payment with invalid json input", TestUpdatePayment_BadInput)
	t.Run("update payment with required attributes missing from input", TestUpdatePayment_MissingAttributes)
	t.Run("update payment with payment id missing from input", TestUpdatePayment_MissingPaymentId)
	t.Run("update payment where nats publish fails", TestUpdatePayment_QueueError)
}

func TestUpdatePayment_Success(t *testing.T) {
	f := SetupAPIServerTest(t)
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
		On("Publish", string(models.UpdatePaymentEvent), mock.Anything).
		Return(nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusAccepted, f.rr.Code)
}

func TestUpdatePayment_BadInput(t *testing.T) {
	f := SetupAPIServerTest(t)
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
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestUpdatePayment_MissingAttributes(t *testing.T) {
	f := SetupAPIServerTest(t)
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
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestUpdatePayment_MissingPaymentId(t *testing.T) {
	f := SetupAPIServerTest(t)
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
	assert.Equal(t, http.StatusBadRequest, f.rr.Code)
}

func TestUpdatePayment_QueueError(t *testing.T) {
	f := SetupAPIServerTest(t)
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
		On("Publish", string(models.UpdatePaymentEvent), mock.Anything).
		Return(errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
}

////////////////////////////////////////

func TestFetchPayment(t *testing.T) {
	t.Run("fetch payment is successful and payment is found", TestFetchPayment_Success)
	t.Run("query service failed to process the fetch request", TestFetchPayment_ServiceError)
	t.Run("fetch payment where organisation id is missing", TestFetchPayment_MissingOrgID)
	t.Run("fetch payment where organisation id is not a valid uuid", TestFetchPayment_InvalidOrgID)
	t.Run("fetch payment where payment id is not a valid uuid", TestFetchPayment_InvalidID)
	t.Run("fetch payment where nats request fails", TestFetchPayment_QueueError)
	t.Run("fetch payment is successful but payment is not found", TestFetchPayment_NotFound)
	t.Run("fetch payment where the event type of the reply is not recognised", TestFetchPayment_UnrecognisedResponse)
}

func TestFetchPayment_Success(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: models.ResourceFoundEvent,
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &models.Payment{},
	})
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusOK, f.rr.Code)
}

func TestFetchPayment_ServiceError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	data := bsonMarshal(t, models.Event{
		EventType: models.ServiceErrorEvent,
		Resource: models.ServiceError{
			Cause:   "service error",
			Request: nil,
		},
	})
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "service error\n", f.rr.Body.String())
}

func TestFetchPayment_MissingOrgID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1//%v", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestFetchPayment_InvalidOrgID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/not-uuid/%v", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestFetchPayment_InvalidID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/not-uuid", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestFetchPayment_QueueError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
}

func TestFetchPayment_NotFound(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: models.ResourceNotFoundEvent,
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &models.Payment{},
	})
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestFetchPayment_UnrecognisedResponse(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: "a man has no name",
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &models.Payment{},
	})
	f.nats.
		On("Request", string(models.FetchPaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "unrecognised response to fetch request: 'a man has no name'\n", f.rr.Body.String())
}

////////////////////////////////////////

func TestDeletePayment(t *testing.T) {
	t.Run("delete payment is successful and payment was found", TestDeletePayment_Success)
	t.Run("query service failed to process the delete request", TestDeletePayment_ServiceError)
	t.Run("delete payment where organisation id is missing", TestDeletePayment_MissingOrgID)
	t.Run("delete payment where organisation id is not a valid uuid", TestDeletePayment_InvalidOrgID)
	t.Run("delete payment where payment id is not a valid uuid", TestDeletePayment_InvalidID)
	t.Run("delete payment where nats request fails", TestDeletePayment_QueueError)
	t.Run("delete payment is successful but payment is not found", TestDeletePayment_NotFound)
	t.Run("delete payment where the event type of the reply is not recognised", TestDeletePayment_UnrecognisedResponse)
}

func TestDeletePayment_Success(t *testing.T) {
	f := SetupAPIServerTest(t)
	locator := models.ResourceLocator{
		OrganisationID: &uuid.Nil,
		ID:             &uuid.Nil,
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: models.ResourceFoundEvent,
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &locator,
	})
	f.nats.
		On("Request", string(models.DeletePaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusGone, f.rr.Code)
}

func TestDeletePayment_ServiceError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	data := bsonMarshal(t, models.Event{
		EventType: models.ServiceErrorEvent,
		Resource: models.ServiceError{
			Cause:   "service error",
			Request: nil,
		},
	})
	f.nats.
		On("Request", string(models.DeletePaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "service error\n", f.rr.Body.String())
}

func TestDeletePayment_MissingOrgID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1//%v", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestDeletePayment_InvalidOrgID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/not-uuid/%v", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestDeletePayment_InvalidID(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/not-uuid", uuid.Nil), nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestDeletePayment_QueueError(t *testing.T) {
	f := SetupAPIServerTest(t)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", uuid.Nil, uuid.Nil), nil)

	f.nats.
		On("Request", string(models.DeletePaymentEvent), mock.Anything, mock.Anything).
		Return(nil, errors.New("queue error"))

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
}

func TestDeletePayment_NotFound(t *testing.T) {
	f := SetupAPIServerTest(t)
	locator := models.ResourceLocator{
		OrganisationID: &uuid.Nil,
		ID:             &uuid.Nil,
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: models.ResourceNotFoundEvent,
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &locator,
	})
	f.nats.
		On("Request", string(models.DeletePaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusNotFound, f.rr.Code)
}

func TestDeletePayment_UnrecognisedResponse(t *testing.T) {
	f := SetupAPIServerTest(t)
	locator := models.ResourceLocator{
		OrganisationID: &uuid.Nil,
		ID:             &uuid.Nil,
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/%v/%v", locator.OrganisationID, locator.ID), nil)

	updateDate := int64(1445444940)
	data := bsonMarshal(t, models.Event{
		EventType: "jaqen h'ghar",
		Version:   2,
		CreatedAt: 499137600,
		UpdatedAt: &updateDate,
		Resource:  &locator,
	})
	f.nats.
		On("Request", string(models.DeletePaymentEvent), mock.Anything, mock.Anything).
		Return(&nats.Msg{
			Subject: "reply",
			Reply:   "",
			Data:    data,
			Sub:     nil,
		}, nil)

	f.server.Router.ServeHTTP(f.rr, req)
	assert.Equal(t, http.StatusInternalServerError, f.rr.Code)
	assert.Equal(t, "unrecognised response to fetch request: 'jaqen h'ghar'\n", f.rr.Body.String())
}
