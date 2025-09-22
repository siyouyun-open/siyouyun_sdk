package ability

import (
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	sdkprotos "github.com/siyouyun-open/siyouyun_sdk/pkg/protos"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

type AI struct {
	conn *grpc.ClientConn
	sdkprotos.AIServiceClient
}

func NewAI() *AI {
	conn, err := grpc.NewClient(utils.GetAIServiceURL(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		sdklog.Logger.Errorf("AI service conn err: %v", err)
		return &AI{}
	}
	return &AI{
		conn:            conn,
		AIServiceClient: sdkprotos.NewAIServiceClient(conn),
	}
}

func (a *AI) Name() string {
	return "AI"
}

func (a *AI) IsReady() bool {
	if a.conn == nil {
		return false
	}
	return a.conn.GetState() == connectivity.Ready
}

func (a *AI) Close() {
	_ = a.conn.Close()
}
