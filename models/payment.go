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

package models

import (
	"cloud.google.com/go/civil"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type Payment struct {
	Type           ResourceType       `json:"type"`
	OrganisationID uuid.UUID          `json:"organisation_id"`
	ID             uuid.UUID          `json:"id"`
	Version        int                `json:"version"`
	Attributes     *PaymentAttributes `json:"attributes"`
}

func (p *Payment) Validate() error {
	if p.Attributes == nil {
		return errors.New("missing payment attributes")
	}
	if p.Attributes.PaymentID == "" {
		return errors.New("missing payment id")
	}
	// TODO: Validate all fields
	return nil
}

type PaymentType string
type SchemePaymentType string
type SchemePaymentSubType string
type Currency string
type AccountType int

type PaymentAttributes struct {
	PaymentID            string               `json:"payment_id"`
	Amount               float32              `json:"amount,string"`
	Currency             Currency             `json:"currency"`
	Purpose              string               `json:"payment_purpose"`
	Scheme               string               `json:"payment_scheme"`
	Type                 PaymentType          `json:"payment_type"`
	ProcessingDate       civil.Date           `json:"processing_date"`
	NumericReference     uint64               `json:"numeric_reference,string"`
	Reference            string               `json:"reference"`
	EndToEndReference    string               `json:"end_to_end_reference"`
	ChargesInformation   ChargesInformation   `json:"charges_information"`
	Exchange             Exchange             `json:"fx"`
	SchemePaymentSubType SchemePaymentSubType `json:"scheme_payment_sub_type"`
	SchemePaymentType    SchemePaymentType    `json:"scheme_payment_type"`
	BeneficiaryParty     Party                `json:"beneficiary_party"`
	DebtorParty          Party                `json:"debtor_party"`
	SponsorParty         Party                `json:"sponsor_party"`
}

type ChargesInformation struct {
	BearerCode              string    `json:"bearer_code"`
	SenderCharges           []Charges `json:"sender_charges"`
	ReceiverChargesAmount   float64   `json:"receiver_charges_amount,string"`
	ReceiverChargesCurrency Currency  `json:"receiver_charges_currency"`
}
type Charges struct {
	Amount   float32  `json:"amount,string"`
	Currency Currency `json:"currency"`
}

type Party struct {
	AccountNumber     string      `json:"account_number"`
	BankId            string      `json:"bank_id"`
	BankIdCode        string      `json:"bank_id_code"`
	Name              string      `json:"name,omitempty"`
	Address           string      `json:"address,omitempty"`
	AccountName       string      `json:"account_name,omitempty"`
	AccountNumberCode string      `json:"account_number_code,omitempty"`
	AccountType       AccountType `json:"account_type,omitempty"`
}

type Exchange struct {
	ContractReference string  `json:"contract_reference"`
	ExchangeRate      float64 `json:"exchange_rate,string"`
	OriginalAmount    float64 `json:"original_amount,string"`
	OriginalCurrency  string  `json:"original_currency"`
}
