package siyouyunsdk

import "github.com/nats-io/nats.go"

var globalNC *nats.Conn

func SetNatsConn(nc *nats.Conn) {
	globalNC = nc
}
