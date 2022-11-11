package gateway

import "fmt"

var mysqlGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, OSHTTPPort, "mysql")
