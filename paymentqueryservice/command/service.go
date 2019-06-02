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
	"strings"

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
	fetchLocators          func(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error)
	buildPaymentFromEvents func([]models.Event) (*models.Event, error)
}

var config = service.Config{}

// NewStart returns a valid Start structure.
func NewStart() *Start {
	return &Start{
		fetchEvents:            fetchEvents,
		fetchLocators:          fetchLocators,
		buildPaymentFromEvents: buildPaymentFromEvents,
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
	// Check that we have someone to reply to
	if msg.Reply == "" {
		return errors.New("reply inbox missing from fetch message")
	}

	// Decode the event
	locator := models.ResourceLocator{}
	if err := bson.Unmarshal(msg.Data, &locator); err != nil {
		return replyWithError(s.Nats, msg, err, "failed to unmarshal create event locator")
	}

	// Get locators list
	locators, err := c.fetchLocators(s.Redis, models.PaymentResource, locator.OrganisationID, locator.ID)
	if err != nil {
		return replyWithError(s.Nats, msg, err, fmt.Sprintf("failed to fetch payment locators for '%v / %v'", scanId(locator.OrganisationID), scanId(locator.ID)))
	}

	// Fetch the payments
	payments := make([]*models.Payment, 0, len(locators))
	for _, locator := range locators {
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

		switch evt.EventType {
		case models.ResourceFoundEvent:
			payments = append(payments, evt.Resource.(*models.Payment))
		case models.ResourceNotFoundEvent:
			// 	Payment was deleted, don't add it
		default:
			log.Errorf("unrecognized event type: '%v'", evt.EventType)
		}
	}

	// Publish the result on the reply subject
	data, err := bson.Marshal(models.Event{
		EventType: models.ResourceFoundEvent,
		Resource:  payments,
	})
	if err != nil {
		return errors.Wrap(err, "failed to encode list request reply")
	}
	if err := s.Nats.Publish(msg.Reply, data); err != nil {
		return replyWithError(s.Nats, msg, err, fmt.Sprintf("failed to post list request reply to '%v'", msg.Reply))
	}

	log.Infof("listed payments '%v / %v'", scanId(locator.OrganisationID), scanId(locator.ID))
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

// updatePayment updates the source payment with data from update.
func updatePayment(source, update *models.Payment) *models.Payment {
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
		c, bb, err := scanEventsKeys(rc, resourceType, organizationID, resourceID, cursor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan event keys")
		}

		for _, b := range bb {
			data, err := rc.Do("GET", b)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to retrieve event data for '%v'", string(b))
			}
			bytes, ok := data.([]byte)
			if !ok {
				return nil, errors.New("redis object data type not recognised")
			}
			log.Debugf("%T", data)

			ev := models.Event{}
			if err := json.Unmarshal(bytes, &ev); err != nil {
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

// fetchLocators returns all the resource locators for the specified organization id and resource id.
// If an id is nil, a wildcard match is used.
func fetchLocators(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID) ([]models.ResourceLocator, error) {
	var (
		cursor   = uint8(0)
		locators = make([]models.ResourceLocator, 0)
	)
	for {
		c, bb, err := scanVersionsKeys(rc, resourceType, organizationID, resourceID, cursor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch versions")
		}

		for _, b := range bb {
			parts := strings.Split(string(b), "/")

			resourceType := models.ResourceType(parts[1])
			organisationID, err := uuid.Parse(parts[2])
			if err != nil {
				log.WithError(err).Errorf("failed to parse uuid '%v'", parts[2])
			}
			resourceID, err := uuid.Parse(parts[3])
			if err != nil {
				log.WithError(err).Errorf("failed to parse uuid '%v'", parts[2])
			}

			locators = append(locators, models.ResourceLocator{
				ResourceType:   &resourceType,
				OrganisationID: &organisationID,
				ID:             &resourceID,
			})
		}

		if c == 0 {
			break
		}
		cursor = c
	}

	return locators, nil
}

// scanEventsKeys returns a batch of events for the specified locator.
func scanEventsKeys(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID, cursor uint8) (uint8, [][]byte, error) {
	scanKey := fmt.Sprintf(models.EventKeyTemplate, resourceType, scanId(organizationID), scanId(resourceID), "*")
	items := make([][]byte, 0)

	resources, err := redis.Values(rc.Do("SCAN", cursor, "MATCH", scanKey))
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to scan '%v' resources '%v/%v'", resourceType, organizationID, resourceID)
	}
	if _, err := redis.Scan(resources, &cursor, &items); err != nil {
		return 0, nil, errors.Wrap(err, "failed to parse events scan")
	}

	return cursor, items, nil
}

// scanVersionsKeys returns a batch of locators keys for the specified locator.
func scanVersionsKeys(rc redis.Conn, resourceType models.ResourceType, organizationID *uuid.UUID, resourceID *uuid.UUID, cursor uint8) (uint8, [][]byte, error) {
	scanKey := fmt.Sprintf(models.VersionKeyTemplate, resourceType, scanId(organizationID), scanId(resourceID))
	items := make([][]byte, 0)

	versions, err := redis.Values(rc.Do("SCAN", cursor, "MATCH", scanKey))
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to scan '%v' resources '%v/%v'", resourceType, organizationID, resourceID)
	}
	if _, err := redis.Scan(versions, &cursor, &items); err != nil {
		return 0, nil, errors.Wrap(err, "failed to parse versions scan")
	}

	return cursor, items, nil
}

// scanId returns the valud of the id if present, or a scan wildcard if nil
func scanId(id *uuid.UUID) string {
	if id == nil {
		return "*"
	}
	return id.String()
}

// replyWithError publish the error to the reply channel and returns the wrapped error.
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
