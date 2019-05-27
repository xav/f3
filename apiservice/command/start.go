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

	"github.com/xav/f3/apiservice/server"
)

// Server starts the dispatch server.
type Server struct {
	server *server.Server
}

var (
	port     int
	natsAddr string
)

// Init returns the runnable cobra command.
func (c *Server) Init() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the api server",
		Run:   c.startServer,
	}

	cmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "The port to listen on.")
	cmd.PersistentFlags().StringVarP(&natsAddr, "nats-addr", "n", nats.DefaultURL, "URL of the NATS server.")

	return cmd
}

func (c *Server) startServer(cmd *cobra.Command, args []string) {
	log.Infof("starting API server")

	// Connect to the NATS server
	nc, err := nats.Connect(natsAddr)
	if err != nil {
		log.WithError(err).Fatalf("failed to connect to NATS server '%v'", natsAddr)
	}

	// Init the API server
	s, err := server.NewServer(func(s *server.Server) error {
		s.Port = port
		s.Nats = nc
		return nil
	})
	if err != nil {
		log.WithError(err).Fatal("error creating server")
	}

	// Start the HTTP handler
	if err := s.Start(); err != nil {
		log.WithError(err).Error("failed to start server")
	}
}
