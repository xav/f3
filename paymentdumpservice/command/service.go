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
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
)

type Start struct {
	natsTimeout time.Duration
}

var config = service.Config{
}

func NewStart() *Start {
	return &Start{
		natsTimeout: 100 * time.Millisecond,
	}
}

// Init returns the runnable cobra command.
func (c *Start) Init() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the payment dump service",
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
		models.DumpPaymentEvent: c.HandleDumpPayment,
	}
	s := service.NewService("f3 payment dumping", handlers)
	if err := s.Start(&config); err != nil {
		log.WithError(err).Error("failed to start service")
	}
}

func (c *Start) HandleDumpPayment(s *service.Service, msg *nats.Msg) error {
	locator := models.ResourceLocator{}
	if err := json.Unmarshal(msg.Data, &locator); err != nil {
		return s.ReplyWithError(msg, err, "failed to unmarshal dump event locator")
	}

	rt := models.PaymentResource
	locatorData, err := json.Marshal(models.ResourceLocator{
		ResourceType:   &rt,
		OrganisationID: locator.OrganisationID,
		ID:             locator.ID,
	})
	if err != nil {
		return s.ReplyWithError(msg, err, "failed to encode payment resource locator")
	}

	reply, err := s.Nats.Request(string(models.FetchPaymentEvent), locatorData, c.natsTimeout)
	if err != nil {
		return s.ReplyWithError(msg, err, "fetch request event failed")
	}
	replyEvent := models.Event{}
	if err := json.Unmarshal(reply.Data, &replyEvent); err != nil {
		return s.ReplyWithError(msg, err, "failed to decode reply locatorData")
	}

	switch replyEvent.EventType {
	case models.ResourceFoundEvent:
		eventKey := fmt.Sprintf("dump/%v/%v/%v/%v", models.PaymentResource, locator.OrganisationID, locator.ID, time.Now().Unix())
		setReply, err := s.Redis.Do("SET", eventKey, replyEvent.Resource)
		if err != nil {
			return s.ReplyWithError(msg, err, "failed to store payment event")
		}
		log.Infof("dumped payment '%v / %v'", locator.OrganisationID, locator.ID)
		if msg.Reply != "" {
			if err = s.Nats.Publish(msg.Reply, []byte(setReply.(string))); err != nil {
				return s.ReplyWithError(msg, err, "failed to reply to request")
			}
		}

	case models.ResourceNotFoundEvent:
		log.Warnf("payment '%v/%v' was not found", locator.OrganisationID.String(), locator.ID.String())
		if msg.Reply != "" {
			evtData, err := json.Marshal(models.Event{
				EventType: models.ResourceNotFoundEvent,
				Resource:  string(locatorData),
			})
			if err != nil {
				return s.ReplyWithError(msg, err, "failed to encode fetch request reply")
			}

			if err = s.Nats.Publish(msg.Reply, evtData); err != nil {
				return s.ReplyWithError(msg, err, "failed to reply to request")
			}
		}

	case models.ServiceErrorEvent:
		err := errors.New("service error")
		return s.ReplyWithError(msg, err, fmt.Sprintf("query service failed to process fetch command: %v", replyEvent.Resource))

	default:
		err := errors.New("unrecognized event type")
		return s.ReplyWithError(msg, err, fmt.Sprintf("unrecognised response to fetch request: '%v'", replyEvent.EventType))
	}

	return nil
}
