// Code generated by mockery v2.40.1. DO NOT EDIT.

package message

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockMessageDispatcher is an autogenerated mock type for the MessageDispatcherInterface type
type MockMessageDispatcher struct {
	mock.Mock
}

// GetClient provides a mock function with given fields: channel
func (_m *MockMessageDispatcher) GetClient(channel MessageChannel) (MessengerClient, error) {
	ret := _m.Called(channel)

	if len(ret) == 0 {
		panic("no return value specified for GetClient")
	}

	var r0 MessengerClient
	var r1 error
	if rf, ok := ret.Get(0).(func(MessageChannel) (MessengerClient, error)); ok {
		return rf(channel)
	}
	if rf, ok := ret.Get(0).(func(MessageChannel) MessengerClient); ok {
		r0 = rf(channel)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(MessengerClient)
		}
	}

	if rf, ok := ret.Get(1).(func(MessageChannel) error); ok {
		r1 = rf(channel)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RegisterClient provides a mock function with given fields: ctx, channel, client
func (_m *MockMessageDispatcher) RegisterClient(ctx context.Context, channel MessageChannel, client MessengerClient) {
	_m.Called(ctx, channel, client)
}

// SendMessage provides a mock function with given fields: ctx, message, channelPriority
func (_m *MockMessageDispatcher) SendMessage(ctx context.Context, message Message, channelPriority []MessageChannel) (MessengerType, error) {
	ret := _m.Called(ctx, message, channelPriority)

	if len(ret) == 0 {
		panic("no return value specified for SendMessage")
	}

	var r0 MessengerType
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, Message, []MessageChannel) (MessengerType, error)); ok {
		return rf(ctx, message, channelPriority)
	}
	if rf, ok := ret.Get(0).(func(context.Context, Message, []MessageChannel) MessengerType); ok {
		r0 = rf(ctx, message, channelPriority)
	} else {
		r0 = ret.Get(0).(MessengerType)
	}

	if rf, ok := ret.Get(1).(func(context.Context, Message, []MessageChannel) error); ok {
		r1 = rf(ctx, message, channelPriority)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockMessageDispatcher creates a new instance of MockMessageDispatcher. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockMessageDispatcher(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockMessageDispatcher {
	mock := &MockMessageDispatcher{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
