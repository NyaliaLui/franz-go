package kgo

import (
	"errors"

	"github.com/twmb/kgo/kerr"
)

type clientErr struct {
	err       error
	retriable bool
}

func (c *clientErr) Error() string {
	return c.err.Error()
}

var (
	errClientClosing         = &clientErr{err: errors.New("client closing")}
	errCorrelationIDMismatch = &clientErr{err: errors.New("correlation ID mismatch")}
	errRecordTooLarge        = &clientErr{err: errors.New("record is too large given client max limits")}
	errBrokerTooOld          = &clientErr{err: errors.New("broker is too old; this client does not support the broker")}

	errNotEnoughData          = &clientErr{err: errors.New("response did not contain enough data to be valid"), retriable: true}
	errInvalidResp            = &clientErr{err: errors.New("invalid response"), retriable: true}
	errNoBrokers              = &clientErr{err: errors.New("all connections to all brokers have died"), retriable: true}
	errNoResp                 = &clientErr{err: errors.New("message was not replied to in a produce response"), retriable: true}
	errBrokerDead             = &clientErr{err: errors.New("broker has been closed"), retriable: true}
	errBrokerConnectionDied   = &clientErr{err: errors.New("broker connection has died"), retriable: true}
	errNoPartitionIDs         = &clientErr{err: errors.New("topic currently has no known partition IDs"), retriable: true}
	errUnknownPartition       = &clientErr{err: errors.New("unknown partition"), retriable: true}
	errUnknownBrokerForLeader = &clientErr{err: errors.New("no broker is known for partition leader id"), retriable: true}
	errUnknownController      = &clientErr{err: errors.New("controller is unknown"), retriable: true}
)

func errIsRetriable(err error) bool {
	switch err := err.(type) {
	case *kerr.Error:
		return kerr.IsRetriable(err)
	case *clientErr:
		return err.retriable
	}
	return false
}

func retriableErr(err error) error {
	return &clientErr{err: err, retriable: true}
}