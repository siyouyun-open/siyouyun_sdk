package ability

import (
	sdkprotos "github.com/siyouyun-open/siyouyun_sdk/pkg/protos"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

type AI struct {
	conn *grpc.ClientConn
	sdkprotos.AIServiceClient
}

func NewAI() *AI {
	conn, err := grpc.Dial(utils.GetAIServiceURL(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[ERROR] AI service conn err: %v", err)
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

func (a *AI) Close() {
	_ = a.conn.Close()
}
