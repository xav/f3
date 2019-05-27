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
	"cloud.google.com/go/civil"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type Payment struct {
	OrganisationID uuid.UUID          `json:"organisation_id" bson:"organisation_id"`
	Attributes     *PaymentAttributes `json:"attributes"      bson:"attributes"`
}

func (p *Payment) Validate() error {
	if p.Attributes == nil {
		return errors.New("missing payment attributes")
	}
	// TODO: Validate all fields
	return nil
}

type PaymentType string
type SchemePaymentType string
type SchemePaymentSubType string
type Currency string
type AccountType int

const (
	DefaultAccountType AccountType = 0
)

type PaymentAttributes struct {
	PaymentID            string               `json:"payment_id"               bson:"payment_id"`
	Amount               float32              `json:"amount,string"            bson:"amount"`
	Currency             Currency             `json:"currency"                 bson:"currency"`
	Purpose              string               `json:"payment_purpose"          bson:"payment_purpose"`
	Scheme               string               `json:"payment_scheme"           bson:"payment_scheme"`
	Type                 PaymentType          `json:"payment_type"             bson:"payment_type"`
	ProcessingDate       civil.Date           `json:"processing_date"          bson:"processing_date"`
	NumericReference     uint64               `json:"numeric_reference,string" bson:"numeric_reference"`
	Reference            string               `json:"reference"                bson:"reference"`
	EndToEndReference    string               `json:"end_to_end_reference"     bson:"end_to_end_reference"`
	ChargesInformation   ChangersInformation  `json:"charges_information"      bson:"charges_information"`
	Exchange             interface{}          `json:"fx"                       bson:"fx"`
	SchemePaymentSubType SchemePaymentSubType `json:"scheme_payment_sub_type"  bson:"scheme_payment_sub_type"`
	SchemePaymentType    SchemePaymentType    `json:"scheme_payment_type"      bson:"scheme_payment_type"`
	BeneficiaryParty     interface{}          `json:"beneficiary_party"        bson:"beneficiary_party"`
	DebtorParty          interface{}          `json:"debtor_party"             bson:"debtor_party"`
	SponsorParty         interface{}          `json:"sponsor_party"            bson:"sponsor_party"`
}

type ChangersInformation struct {
	BearerCode              string    `json:"bearer_code"                    bson:"bearer_code"`
	SenderCharges           []Charges `json:"sender_charges"                 bson:"sender_charges"`
	ReceiverChargesAmount   float64   `json:"receiver_charges_amount,string" bson:"receiver_charges_amount"`
	ReceiverChargesCurrency Currency  `json:"receiver_charges_currency"      bson:"receiver_charges_currency"`
}
type Charges struct {
	Amount   float32  `json:"amount,string" bson:"amount"`
	Currency Currency `json:"currency"      bson:"currency"`
}

type Party struct {
	AccountNumber     string      `json:"account_number"                bson:"account_number"`
	BankId            string      `json:"bank_id"                       bson:"bank_id"`
	BankIdCode        string      `json:"bank_id_code"                  bson:"bank_id_code"`
	Name              string      `json:"name,omitempty"                bson:"name,omitempty"`
	Address           string      `json:"address,omitempty"             bson:"address,omitempty"`
	AccountName       string      `json:"account_name,omitempty"        bson:"account_name,omitempty"`
	AccountNumberCode string      `json:"account_number_code,omitempty" bson:"account_number_code,omitempty"`
	AccountType       AccountType `json:"account_type,omitempty"        bson:"account_type,omitempty"`
}
