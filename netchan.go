// Copyright 2013-2014 Apcera Inc. All rights reserved.

package nats

import (
	"errors"
	"reflect"
)

// This allows the functionality for network channels by binding send and receive Go chans
// to subjects and optionally queue groups.
// Data will be encoded and decoded via the EncodedConn and its associated encoders.

// Bind a channel for send operations to nats.
func (c *EncodedConn) BindSendChan(subject string, channel interface{}) error {
	chVal := reflect.ValueOf(channel)
	if chVal.Kind() != reflect.Chan {
		return ErrChanArg
	}
	go chPublish(c, chVal, subject)
	return nil
}

// Publish all values that arrive on the channel until it is closed or we
// ecounter an error.
func chPublish(c *EncodedConn, chVal reflect.Value, subject string) {
	for {
		val, ok := chVal.Recv()
		if !ok {
			// Channel has most likely been closed.
			return
		}
		if e := c.Publish(subject, val.Interface()); e != nil {
			if c.Conn.Opts.AsyncErrorCB != nil {
				// FIXME(dlc) - Not sure this is the right thing to do.
				go c.Conn.Opts.AsyncErrorCB(c.Conn, nil, e)
			}
			return
		}
	}
}

// Bind a channel for receive operations from nats.
func (c *EncodedConn) BindRecvChan(subject string, channel interface{}) (*Subscription, error) {
	return c.bindRecvChan(subject, _EMPTY_, channel)
}

// Bind a channel for queue-based receive operations from nats.
func (c *EncodedConn) BindRecvQueueChan(subject, queue string, channel interface{}) (*Subscription, error) {
	return c.bindRecvChan(subject, queue, channel)
}

// Internal function to bind receive operations for a channel.
func (c *EncodedConn) bindRecvChan(subject, queue string, channel interface{}) (*Subscription, error) {
	chVal := reflect.ValueOf(channel)
	if chVal.Kind() != reflect.Chan {
		return nil, ErrChanArg
	}
	argType := chVal.Type().Elem()

	cb := func(m *Msg) {
		var oPtr reflect.Value
		if argType.Kind() != reflect.Ptr {
			oPtr = reflect.New(argType)
		} else {
			oPtr = reflect.New(argType.Elem())
		}
		if err := c.Enc.Decode(m.Subject, m.Data, oPtr.Interface()); err != nil {
			c.Conn.err = errors.New("nats: Got an error trying to unmarshal: " + err.Error())
			if c.Conn.Opts.AsyncErrorCB != nil {
				go c.Conn.Opts.AsyncErrorCB(c.Conn, m.Sub, c.Conn.err)
			}
			return
		}
		if argType.Kind() != reflect.Ptr {
			oPtr = reflect.Indirect(oPtr)
		}
		// This is a bit hacky, but in this instance we may be trying to send to a closed channel.
		// and the user does not know when it is safe to close the channel.
		defer func() {
			// If we have panicked, recover and close the subscription.
			if r := recover(); r != nil {
				m.Sub.Unsubscribe()
			}
		}()
		// Actually do the send to the channel.
		chVal.Send(oPtr)
	}

	return c.Conn.subscribe(subject, queue, cb)
}
