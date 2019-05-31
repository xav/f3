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

type EventType string

const (
	CreatePaymentEvent EventType = "payment:create"
	UpdatePaymentEvent EventType = "payment:update"
	DeletePaymentEvent EventType = "payment:delete"

	FetchPaymentEvent EventType = "payment:fetch"
	ListPaymentEvent  EventType = "payment:list"
	DumpPaymentEvent  EventType = "payment:dump"

	PaymentFoundEvent    EventType = "payment:found"
	PaymentNotFoundEvent EventType = "payment:notfound"
)

const (
	VersionKeyTemplate   = "v/%v/%v/%v"
	EventKeyTemplate     = "e/%v/%v/%v/%v"
	EventKeyScanTemplate = "e/%v/%v/%v/*"
)

type StoreEvent struct {
	EventType EventType   `json:"event_type" bson:"event_type"`
	Version   int64       `json:"version"    bson:"version"`
	CreatedAt int64       `json:"created_at" bson:"created_at"`
	Resource  interface{} `json:"resource"   bson:"resource"`
}
