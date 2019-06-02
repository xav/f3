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
	"encoding/json"
	"fmt"
	"sort"

	"github.com/apex/log"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/xav/f3/f3nats"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
	"gopkg.in/mgo.v2/bson"
)

type Start struct {
	fetchEvents            func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID) ([]models.Event, error)
	buildPaymentFromEvents func([]models.Event) (*models.Event, error)
	scanResources          func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error)
}

var config = service.Config{}

// NewStart returns a valid Start structure.
func NewStart() *Start {
	return &Start{
		fetchEvents:            fetchEvents,
		buildPaymentFromEvents: buildPaymentFromEvents,
		scanResources:          scanResources,
	}
}

// Init returns the runnable cobra command.
func (c *Start) Init() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the payment query service",
		Run:   c.start,
	}

	cmd.PersistentFlags().StringVarP(&config.RedisURL, "redis-url", "r", "redis://localhost:6379", "Redis server URL.")
	cmd.PersistentFlags().StringVarP(&config.NatsURL, "nats-url", "n", nats.DefaultURL, "The NATS server URLs (separated by comma).")
	cmd.PersistentFlags().StringVarP(&config.NatsUserCreds, "nats-creds", "c", "", "NATS User Credentials File.")
	cmd.PersistentFlags().StringVarP(&config.NatsKeyFile, "nats-nkey", "k", "", "NATS NKey Seed File.")

	return cmd
}

func (c *Start) start(cmd *cobra.Command, args []string) {
	handlers := map[models.EventType]service.MsgHandler{
		models.FetchPaymentEvent: c.HandleFetchPayment,
		models.ListPaymentEvent:  c.HandleListPayment,
	}
	s := service.NewService("f3 payment querying", handlers)
	if err := s.Start(&config); err != nil {
		log.WithError(err).Error("failed to start service")
	}
}

func (c *Start) HandleFetchPayment(s *service.Service, msg *nats.Msg) error {
	// Check that we have someone to reply to
	if msg.Reply == "" {
		return errors.New("reply inbox missing from fetch message")
	}

	// Decode the event
	locator := models.ResourceLocator{}
	if err := bson.Unmarshal(msg.Data, &locator); err != nil {
		return replyWithError(s.Nats, msg, err, "failed to unmarshal create event locator")
	}

	// Get payment resource history
	evts, err := c.fetchEvents(s.Redis, models.PaymentResource, locator.OrganisationID, locator.ID)
	if err != nil {
		return replyWithError(s.Nats, msg, err, fmt.Sprintf("failed to fetch payment events for '%v / %v'", locator.OrganisationID, locator.ID))
	}

	// Apply the events
	evt, err := c.buildPaymentFromEvents(evts)
	if err != nil {
		return replyWithError(s.Nats, msg, err, fmt.Sprintf("failed to build payment from events for '%v / %v'", locator.OrganisationID, locator.ID))
	}

	// Publish the result on the reply subject
	data, err := bson.Marshal(evt)
	if err != nil {
		return errors.Wrap(err, "failed to encode fetch request reply")
	}
	if err := s.Nats.Publish(msg.Reply, data); err != nil {
		return replyWithError(s.Nats, msg, err, fmt.Sprintf("failed to post fetch request reply to '%v'", msg.Reply))
	}

	log.Infof("fetched payment '%v / %v'", locator.OrganisationID, locator.ID)
	return nil
}

func (c *Start) HandleListPayment(s *service.Service, msg *nats.Msg) error {
	return nil
}

// buildPaymentFromEvents applies the event in chronological order and returns
// the result as an either a 'found' event with the payment resource,
// or a 'not found' event.
func buildPaymentFromEvents(events []models.Event) (*models.Event, error) {
	// We don't need to continue of we don't have any events to apply.
	if len(events) == 0 {
		return &models.Event{
			EventType: models.ResourceNotFoundEvent,
		}, nil
	}

	// The first event of the sequence should always be 'create',
	if events[0].EventType != models.CreatePaymentEvent {
		return nil, errors.New("invalid events sequence: sequence should start with a create event")
	}
	payment, ok := events[0].Resource.(*models.Payment)
	if !ok {
		return nil, errors.New("invalid event: the create event resource is not 'Payment'")
	}

	// Initialise the start state of the return event.
	evt := &models.Event{
		EventType: models.ResourceFoundEvent,
		Version:   events[0].Version,
		CreatedAt: events[0].CreatedAt,
		UpdatedAt: &events[0].CreatedAt,
		Resource:  payment,
	}

	// Apply the rest of the events and update the return event accordingly.
	for _, ev := range events[1:] {
		switch ev.EventType {
		case models.UpdatePaymentEvent:
			update, ok := ev.Resource.(*models.Payment)
			if !ok {
				return nil, errors.New("invalid event: the update event resource is not 'Payment'")
			}
			payment = updatePayment(payment, update)
			evt.Resource = payment
			evt.Version = ev.Version
			evt.UpdatedAt = &ev.CreatedAt
		case models.DeletePaymentEvent:
			evt.EventType = models.ResourceNotFoundEvent
			evt.Version = ev.Version
			evt.UpdatedAt = &ev.CreatedAt
			evt.Resource = nil
			return evt, nil
		default:
			return nil, errors.Errorf("unrecognised event type: '%v'", ev.EventType)
		}
	}

	return evt, nil
}

func updatePayment(_, update *models.Payment) *models.Payment {
	// TODO(xav): apply diffs from update to the original payment instead of replacing it.
	p := *update
	return &p
}

// fetchEvents returns all the events for the specified resource, in chronological order.
func fetchEvents(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.Event, error) {
	var (
		cursor = uint8(0)
		events = make([]models.Event, 0)
	)
	for {
		c, bb, err := scanResources(rc, resourceType, organizationID, resourceID, cursor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch resource events")
		}

		for _, b := range bb {
			ev := models.Event{}
			if err := json.Unmarshal(b, &ev); err != nil {
				return nil, errors.Wrap(err, "failed to parse event data")
			}
			events = append(events, ev)
		}

		if c == 0 {
			break
		}
		cursor = c
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Version < events[j].Version
	})

	return events, nil
}

func scanResources(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID, cursor uint8) (uint8, [][]byte, error) {
	getScanId := func(id *uuid.UUID) string {
		if id == nil {
			return "*"
		}
		return id.String()
	}
	scanKey := fmt.Sprintf(models.EventKeyTemplate, resourceType, getScanId(organizationID), getScanId(resourceID), "*")
	items := make([][]byte, 0)

	existing, err := redis.Values(rc.Do("SCAN", cursor, "MATCH", scanKey))
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to scan '%v' resources '%v/%v'", resourceType, organizationID, resourceID)
	}
	if _, err := redis.Scan(existing, &cursor, &items); err != nil {
		return 0, nil, errors.Wrap(err, "failed to parse resources list")
	}

	return cursor, items, nil
}

func replyWithError(conn f3nats.NatsConn, request *nats.Msg, err error, msg string) error {
	if request == nil {
		log.Error("request cannot be nil for reply")
		return errors.WithMessage(errors.Wrap(err, msg), "request cannot be mil for reply")
	}
	se := models.ServiceError{
		Cause:   msg,
		Request: request,
	}

	if data, err := bson.Marshal(se); err != nil {
		log.Errorf("failed to marshal service error (%v)", msg)
		if err := conn.Publish(request.Reply, []byte(msg)); err != nil {
			log.Errorf("failed to post error reply to '%v'", request.Reply)
		}
	} else {
		if err := conn.Publish(request.Reply, data); err != nil {
			log.Errorf("failed to post error reply to '%v'", request.Reply)
		}
	}

	return errors.Wrap(err, msg)
}
