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
	"time"

	"github.com/apex/log"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
)

type Start struct {
	scanVersionsKeys func(redis.Conn, models.ResourceType, *uuid.UUID, *uuid.UUID, uint8) (uint8, [][]byte, error)
}

var config = service.Config{}

// NewStart returns a valid Start structure.
func NewStart() *Start {
	return &Start{
		scanVersionsKeys: scanVersionsKeys,
	}
}

// Init returns the runnable cobra command.
func (c *Start) Init() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the payment processing service",
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
		models.CreatePaymentEvent: c.HandleCreatePayment,
		models.UpdatePaymentEvent: c.HandleUpdatePayment,
		models.DeletePaymentEvent: c.HandleDeletePayment,
	}
	s := service.NewService("f3 payment processing", handlers)
	if err := s.Start(&config); err != nil {
		log.WithError(err).Error("failed to start service")
	}
}

func (c *Start) HandleCreatePayment(s *service.Service, msg *nats.Msg) error {
	// Decode the event
	payment := models.Payment{}
	if err := json.Unmarshal(msg.Data, &payment); err != nil {
		return errors.Wrap(err, "failed to unmarshal create payment event")
	}

	// Check if the payment is already present
	_, keys, err := c.scanVersionsKeys(s.Redis, models.PaymentResource, &payment.OrganisationID, &payment.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check existing payments")
	}
	if len(keys) > 0 {
		return errors.New("payment id already present in store")
	}

	// Create the version index
	versionKey := fmt.Sprintf(models.VersionKeyTemplate, models.PaymentResource, payment.OrganisationID, payment.ID)
	version, err := s.Redis.Do("INCR", versionKey)
	if err != nil {
		return errors.Wrap(err, "failed to increment aggregate version")
	}

	// Save the event to the store
	paymentBytes, err := json.Marshal(payment)
	if err != nil {
		return errors.Wrap(err, "failed to marshal payment data")
	}
	bytes, err := json.Marshal(models.Event{
		EventType: models.CreatePaymentEvent,
		Version:   version.(int64),
		CreatedAt: time.Now().Unix(),
		Resource:  string(paymentBytes),
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal event data")
	}
	eventKey := fmt.Sprintf(models.EventKeyTemplate, models.PaymentResource, payment.OrganisationID, payment.ID, version)
	reply, err := s.Redis.Do("SET", eventKey, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to store payment event")
	}

	// Reply if needed
	if msg.Reply != "" {
		if err = s.Nats.Publish(msg.Reply, []byte(reply.(string))); err != nil {
			return errors.Wrap(err, "failed to reply to request")
		}
	}

	log.Infof("created payment '%v / %v'", payment.OrganisationID, payment.ID)
	return nil
}

func (c *Start) HandleUpdatePayment(s *service.Service, msg *nats.Msg) error {
	// Decode the event
	payment := models.Payment{}
	if err := json.Unmarshal(msg.Data, &payment); err != nil {
		return errors.Wrap(err, "failed to unmarshal update payment event")
	}

	// Check if the payment is already present
	_, keys, err := c.scanVersionsKeys(s.Redis, models.PaymentResource, &payment.OrganisationID, &payment.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check existing payments")
	}
	if len(keys) == 0 {
		return errors.New("payment id was not found in store")
	}

	// Increment the version index
	versionKey := fmt.Sprintf(models.VersionKeyTemplate, models.PaymentResource, payment.OrganisationID, payment.ID)
	version, err := s.Redis.Do("INCR", versionKey)
	if err != nil {
		return errors.Wrap(err, "failed to increment aggregate version")
	}

	// Save the event to the store
	paymentBytes, err := json.Marshal(payment)
	if err != nil {
		return errors.Wrap(err, "failed to marshal payment data")
	}
	bytes, err := json.Marshal(models.Event{
		EventType: models.UpdatePaymentEvent,
		Version:   version.(int64),
		CreatedAt: time.Now().Unix(),
		Resource:  string(paymentBytes),
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal event data")
	}
	eventKey := fmt.Sprintf(models.EventKeyTemplate, models.PaymentResource, payment.OrganisationID, payment.ID, version)
	reply, err := s.Redis.Do("SET", eventKey, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to store payment event")
	}

	// Reply if needed
	if msg.Reply != "" {
		if err = s.Nats.Publish(msg.Reply, []byte(reply.(string))); err != nil {
			return errors.Wrap(err, "failed to reply to request")
		}
	}

	log.Infof("update payment '%v / %v' (%v)", payment.OrganisationID, payment.ID, version)
	return nil
}

func (c *Start) HandleDeletePayment(s *service.Service, msg *nats.Msg) error {
	// Decode the event
	locator := models.ResourceLocator{}
	if err := json.Unmarshal(msg.Data, &locator); err != nil {
		return errors.Wrap(err, "failed to unmarshal delete payment event")
	}

	// Check if the payment is already present
	_, keys, err := c.scanVersionsKeys(s.Redis, models.PaymentResource, locator.OrganisationID, locator.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check existing payments")
	}
	if len(keys) == 0 {
		return errors.New("payment id was not found in store")
	}

	// Increment the version index
	versionKey := fmt.Sprintf(models.VersionKeyTemplate, models.PaymentResource, locator.OrganisationID, locator.ID)
	version, err := s.Redis.Do("INCR", versionKey)
	if err != nil {
		return errors.Wrap(err, "failed to increment aggregate version")
	}

	// Save the event to the store
	locatorBytes, err := json.Marshal(locator)
	if err != nil {
		return errors.Wrap(err, "failed to marshal payment data")
	}
	bytes, err := json.Marshal(models.Event{
		EventType: models.DeletePaymentEvent,
		Version:   version.(int64),
		CreatedAt: time.Now().Unix(),
		Resource:  string(locatorBytes),
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal event data")
	}
	eventKey := fmt.Sprintf(models.EventKeyTemplate, models.PaymentResource, locator.OrganisationID, locator.ID, version)
	reply, err := s.Redis.Do("SET", eventKey, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to store payment event")
	}

	// Reply if needed
	if msg.Reply != "" {
		if err = s.Nats.Publish(msg.Reply, []byte(reply.(string))); err != nil {
			return errors.Wrap(err, "failed to reply to request")
		}
	}

	log.Infof("delete payment '%v / %v' (%v)", locator.OrganisationID, locator.ID, version)
	return nil
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
