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
	ServiceErrorEvent EventType = "service:error"

	ResourceFoundEvent    EventType = "resource:found"
	ResourceNotFoundEvent EventType = "resource:notfound"

	CreatePaymentEvent EventType = "payment:create"
	UpdatePaymentEvent EventType = "payment:update"
	DeletePaymentEvent EventType = "payment:delete"

	FetchPaymentEvent EventType = "payment:fetch"
	ListPaymentEvent  EventType = "payment:list"
	DumpPaymentEvent  EventType = "payment:dump"

	PaymentCreatedEvent EventType = "payment:created"
	PaymentUpdatedEvent EventType = "payment:updated"
	PaymentDeletedEvent EventType = "payment:deleted"
	PaymentDumpedEvent  EventType = "payment:dumped"
)

const (
	VersionKeyTemplate = "v/%v/%v/%v"
	EventKeyTemplate   = "e/%v/%v/%v/%v"
)

// Event is the generic event holder
type Event struct {
	EventType EventType `json:"event_type"`
	Version   int64     `json:"version"`
	CreatedAt int64     `json:"created_at"`
	UpdatedAt *int64    `json:"updated_at"`
	Resource  string    `json:"resource"`
}
