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

package f3nats

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

//go:generate mockery -name NatsConn

type NatsConn interface {
	SetDisconnectHandler(dcb nats.ConnHandler)
	SetReconnectHandler(rcb nats.ConnHandler)
	SetDiscoveredServersHandler(dscb nats.ConnHandler)
	SetClosedHandler(cb nats.ConnHandler)
	SetErrorHandler(cb nats.ErrHandler)
	ConnectedUrl() string
	ConnectedAddr() string
	ConnectedServerId() string
	LastError() error
	Publish(subj string, data []byte) error
	PublishMsg(m *nats.Msg) error
	PublishRequest(subj, reply string, data []byte) error
	Request(subj string, data []byte, timeout time.Duration) (*nats.Msg, error)
	NewRespInbox() string
	Subscribe(subj string, cb nats.MsgHandler) (*nats.Subscription, error)
	ChanSubscribe(subj string, ch chan *nats.Msg) (*nats.Subscription, error)
	ChanQueueSubscribe(subj, group string, ch chan *nats.Msg) (*nats.Subscription, error)
	SubscribeSync(subj string) (*nats.Subscription, error)
	QueueSubscribe(subj, queue string, cb nats.MsgHandler) (*nats.Subscription, error)
	QueueSubscribeSync(subj, queue string) (*nats.Subscription, error)
	QueueSubscribeSyncWithChan(subj, queue string, ch chan *nats.Msg) (*nats.Subscription, error)
	NumSubscriptions() int
	FlushTimeout(timeout time.Duration) (err error)
	Flush() error
	Buffered() (int, error)
	Close()
	IsClosed() bool
	IsReconnecting() bool
	IsConnected() bool
	Drain() error
	IsDraining() bool
	Servers() []string
	DiscoveredServers() []string
	Status() nats.Status
	Stats() nats.Statistics
	MaxPayload() int64
	AuthRequired() bool
	TLSRequired() bool
	Barrier(f func()) error
	GetClientID() (uint64, error)
	RequestWithContext(ctx context.Context, subj string, data []byte) (*nats.Msg, error)
	FlushWithContext(ctx context.Context) error
}
