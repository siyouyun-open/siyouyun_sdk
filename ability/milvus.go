package ability

import (
	"context"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/milvus-io/milvus-sdk-go/v2/merr"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"sync"
	"time"
)

// Milvus vector database
type Milvus struct {
	appCode *string
	client.Client
	expireTimerMap sync.Map
}

func NewMilvus(appCode *string) (*Milvus, error) {
	var c client.Client
	var err error
	var maxRetry = 10
	for i := 0; i < maxRetry; i++ {
		c, err = client.NewDefaultGrpcClient(
			context.Background(),
			sdkconst.MilvusServiceURL,
		)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 3)
	}
	if err != nil {
		return nil, err
	}
	return &Milvus{appCode: appCode, Client: c}, nil
}

func (m *Milvus) Name() string {
	return "Milvus"
}

func (m *Milvus) Close() {
	_ = m.Client.Close()
}

// GenCollectionName generate the collection name
func (m *Milvus) GenCollectionName(buzName string) string {
	return *m.appCode + "_" + buzName
}

// GenPartitionName generate the partition name
func (m *Milvus) GenPartitionName(ugn *utils.UserGroupNamespace) string {
	return ugn.DatabaseName()
}

// LoadAppCollection load app collection and set the expiration time
func (m *Milvus) LoadAppCollection(ugn *utils.UserGroupNamespace, collectionName string, expireTime ...time.Duration) error {
	if m.Client == nil {
		return merr.ErrServiceUnavailable
	}
	partitionNames := []string{m.GenPartitionName(ugn)}
	state, err := m.GetLoadState(context.Background(), collectionName, partitionNames)
	if err != nil {
		return err
	}
	if state == entity.LoadStateNotExist {
		return merr.ErrPartitionNotFound
	}

	// finally set the collection expiration time
	defer func() {
		if len(expireTime) > 0 {
			m.setExpireTimer(ugn, collectionName, expireTime[0])
		}
	}()

	if state == entity.LoadStateLoaded {
		return nil
	}
	if state == entity.LoadStateLoading {
		// waiting collection loaded
		var maxWaitCount = 10
		for i := 0; i < maxWaitCount; i++ {
			state, err = m.GetLoadState(context.Background(), collectionName, partitionNames)
			if err != nil {
				return err
			}
			if state == entity.LoadStateLoaded {
				return nil
			}
			time.Sleep(3 * time.Second)
		}
		return merr.ErrPartitionNotFullyLoaded
	}
	if state == entity.LoadStateNotLoad {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return m.Client.LoadPartitions(ctx, collectionName, partitionNames, false)
	}
	return nil
}

func (m *Milvus) setExpireTimer(ugn *utils.UserGroupNamespace, collectionName string, d time.Duration) {
	go func() {
		key := ugn.String()
		if t, ok := m.expireTimerMap.Load(key); ok {
			expireTimer := t.(*time.Timer)
			if !expireTimer.Stop() {
				<-expireTimer.C
			}
			expireTimer.Reset(d)
		} else {
			expireTimer := time.AfterFunc(d, func() {
				_ = m.ReleasePartitions(context.Background(), collectionName, []string{ugn.DatabaseName()})
				m.expireTimerMap.Delete(key)
			})
			m.expireTimerMap.Store(key, expireTimer)
		}
	}()
}
