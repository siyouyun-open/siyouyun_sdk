package ability

import (
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

type AI struct {
	conn *grpc.ClientConn
	protos.AIServiceClient
}

func NewAI() *AI {
	conn, err := grpc.NewClient(sdkconst.AIServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[ERROR] AI service conn err: %v", err)
		return nil
	}
	return &AI{
		conn:            conn,
		AIServiceClient: protos.NewAIServiceClient(conn),
	}
}

func (a *AI) Name() string {
	return "AI"
}

func (a *AI) Close() {
	_ = a.conn.Close()
}
