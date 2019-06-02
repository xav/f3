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
	ResourceFoundEvent    EventType = "resource:found"
	ResourceNotFoundEvent EventType = "resource:notfound"

	CreatePaymentEvent EventType = "payment:create"
	UpdatePaymentEvent EventType = "payment:update"
	DeletePaymentEvent EventType = "payment:delete"

	FetchPaymentEvent EventType = "payment:fetch"
	ListPaymentEvent  EventType = "payment:list"
	DumpPaymentEvent  EventType = "payment:dump"
)

const (
	VersionKeyTemplate = "v/%v/%v/%v"
	EventKeyTemplate   = "e/%v/%v/%v/%v"
)

// Event is the generic event holder
type Event struct {
	EventType EventType   `json:"event_type" bson:"event_type"`
	Version   int64       `json:"version"    bson:"version"`
	CreatedAt int64       `json:"created_at" bson:"created_at"`
	UpdatedAt *int64      `json:"updated_at" bson:"updated_at"`
	Resource  interface{} `json:"resource"   bson:"resource"`
}

// PaymentEvent is a specialisation of Event used for bson deserialization
type PaymentEvent struct {
	EventType EventType `json:"event_type" bson:"event_type"`
	Version   int64     `json:"version"    bson:"version"`
	CreatedAt int64     `json:"created_at" bson:"created_at"`
	UpdatedAt *int64    `json:"updated_at" bson:"updated_at"`
	Resource  *Payment  `json:"resource"   bson:"resource"`
}
