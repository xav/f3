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
	"github.com/apex/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"github.com/xav/f3/models"
	"github.com/xav/f3/service"
)

type Start struct {
}

var config = service.Config{}

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
		models.FetchPaymentEvent: HandleFetchPayment,
		models.ListPaymentEvent:  HandleListPayment,
	}
	s := service.NewService("f3 payment querying", handlers)
	if err := s.Start(&config); err != nil {
		log.WithError(err).Error("failed to start service")
	}
}

func HandleFetchPayment(s *service.Service, msg *nats.Msg) error {
	return nil
}

func HandleListPayment(s *service.Service, msg *nats.Msg) error {
	return nil
}
