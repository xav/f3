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
	"time"

	"github.com/apex/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"github.com/xav/f3/apiserver/server"
)

// Server starts the dispatch server.
type Server struct {
	server *server.Server
}

type UserConfig struct {
	port          int
	natsURL       string
	natsUserCreds string
	natsKeyFile   string
}

var config = UserConfig{}

// Init returns the runnable cobra command.
func (c *Server) Init() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the api server",
		Run:   c.startServer,
	}

	cmd.PersistentFlags().IntVarP(&config.port, "port", "p", 8080, "The port to listen on.")
	cmd.PersistentFlags().StringVarP(&config.natsURL, "nats-url", "n", nats.DefaultURL, "The NATS server URLs (separated by comma).")
	cmd.PersistentFlags().StringVarP(&config.natsUserCreds, "nats-creds", "c", "", "NATS User Credentials File.")
	cmd.PersistentFlags().StringVarP(&config.natsKeyFile, "nats-nkey", "k", "", "NATS NKey Seed File.")

	return cmd
}

func (c *Server) startServer(cmd *cobra.Command, args []string) {

	// Init the API server
	s, err := server.NewServer(func(s *server.Server) error {
		s.Port = config.port
		return nil
	}, openNatsConnection)
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}

	// Start the HTTP handler
	if err := s.Start(); err != nil {
		log.WithError(err).Error("failed to start server")
	}
}

func openNatsConnection(s *server.Server) error {
	// Connect Options.
	opts := []nats.Option{nats.Name("f3 payment API")}
	opts = setupNatsConnOptions(opts)

	// Use UserCredentials
	if config.natsUserCreds != "" {
		opts = append(opts, nats.UserCredentials(config.natsUserCreds))
	}

	// Use Nkey authentication.
	if config.natsKeyFile != "" {
		opt, err := nats.NkeyOptionFromSeed(config.natsKeyFile)
		if err != nil {
			log.WithError(err).Fatal("failed to load nats seed file")
		}
		opts = append(opts, opt)
	}

	// Connect to NATS
	log.Infof("connecting to nats")
	nc, err := nats.Connect(config.natsURL, opts...)
	if err != nil {
		log.WithError(err).Fatal("failed to connect to NATS. make sure the nats server is running")
	}

	s.Nats = nc
	return nil
}

func setupNatsConnOptions(opts []nats.Option) []nats.Option {
	var (
		totalWait      = 5 * time.Minute
		reconnectDelay = time.Second
	)
	opts = append(opts, nats.ReconnectWait(reconnectDelay))
	opts = append(opts, nats.MaxReconnects(int(totalWait/reconnectDelay)))
	opts = append(opts, nats.DisconnectHandler(func(nc *nats.Conn) {
		log.Infof("Disconnected: will attempt reconnects for %.0fm", totalWait.Minutes())
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		log.Infof("Reconnected [%s]", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		log.Fatal("Exiting, no servers available")
	}))
	return opts
}
