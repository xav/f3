// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import context "context"

import mock "github.com/stretchr/testify/mock"
import nats "github.com/nats-io/nats.go"
import time "time"

// NatsSubscription is an autogenerated mock type for the NatsSubscription type
type NatsSubscription struct {
	mock.Mock
}

// AutoUnsubscribe provides a mock function with given fields: max
func (_m *NatsSubscription) AutoUnsubscribe(max int) error {
	ret := _m.Called(max)

	var r0 error
	if rf, ok := ret.Get(0).(func(int) error); ok {
		r0 = rf(max)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ClearMaxPending provides a mock function with given fields:
func (_m *NatsSubscription) ClearMaxPending() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Delivered provides a mock function with given fields:
func (_m *NatsSubscription) Delivered() (int64, error) {
	ret := _m.Called()

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Drain provides a mock function with given fields:
func (_m *NatsSubscription) Drain() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Dropped provides a mock function with given fields:
func (_m *NatsSubscription) Dropped() (int, error) {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// IsValid provides a mock function with given fields:
func (_m *NatsSubscription) IsValid() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MaxPending provides a mock function with given fields:
func (_m *NatsSubscription) MaxPending() (int, int, error) {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 int
	if rf, ok := ret.Get(1).(func() int); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(int)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// NextMsg provides a mock function with given fields: timeout
func (_m *NatsSubscription) NextMsg(timeout time.Duration) (*nats.Msg, error) {
	ret := _m.Called(timeout)

	var r0 *nats.Msg
	if rf, ok := ret.Get(0).(func(time.Duration) *nats.Msg); ok {
		r0 = rf(timeout)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*nats.Msg)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(time.Duration) error); ok {
		r1 = rf(timeout)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NextMsgWithContext provides a mock function with given fields: ctx
func (_m *NatsSubscription) NextMsgWithContext(ctx context.Context) (*nats.Msg, error) {
	ret := _m.Called(ctx)

	var r0 *nats.Msg
	if rf, ok := ret.Get(0).(func(context.Context) *nats.Msg); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*nats.Msg)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Pending provides a mock function with given fields:
func (_m *NatsSubscription) Pending() (int, int, error) {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 int
	if rf, ok := ret.Get(1).(func() int); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(int)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// PendingLimits provides a mock function with given fields:
func (_m *NatsSubscription) PendingLimits() (int, int, error) {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 int
	if rf, ok := ret.Get(1).(func() int); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(int)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// QueuedMsgs provides a mock function with given fields:
func (_m *NatsSubscription) QueuedMsgs() (int, error) {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SetPendingLimits provides a mock function with given fields: msgLimit, bytesLimit
func (_m *NatsSubscription) SetPendingLimits(msgLimit int, bytesLimit int) error {
	ret := _m.Called(msgLimit, bytesLimit)

	var r0 error
	if rf, ok := ret.Get(0).(func(int, int) error); ok {
		r0 = rf(msgLimit, bytesLimit)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Type provides a mock function with given fields:
func (_m *NatsSubscription) Type() nats.SubscriptionType {
	ret := _m.Called()

	var r0 nats.SubscriptionType
	if rf, ok := ret.Get(0).(func() nats.SubscriptionType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(nats.SubscriptionType)
	}

	return r0
}

// Unsubscribe provides a mock function with given fields:
func (_m *NatsSubscription) Unsubscribe() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}