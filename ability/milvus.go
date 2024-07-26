package ability

import (
	"context"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/milvus-io/milvus-sdk-go/v2/merr"
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
			utils.GetMilvusServiceURL(),
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
func (m *Milvus) LoadAppCollection(ugn *utils.UserGroupNamespace, collectionName string, expireTime ...time.Duration) (err error) {
	if m.Client == nil {
		return merr.ErrServiceUnavailable
	}

	// finally set the collection expiration time
	defer func() {
		if len(expireTime) > 0 && err == nil {
			m.setExpireTimer(ugn, collectionName, expireTime[0])
		}
	}()

	var partitionNames = []string{m.GenPartitionName(ugn)}
	var maxLoopCount = 10
	var state entity.LoadState
	for i := 0; i < maxLoopCount; i++ {
		state, err = m.GetLoadState(context.Background(), collectionName, partitionNames)
		if err != nil {
			return
		}
		switch state {
		case entity.LoadStateLoaded:
			// partition already loaded
			return
		case entity.LoadStateNotExist:
			// partition not found
			err = m.CreatePartition(context.Background(), collectionName, m.GenPartitionName(ugn))
			if err != nil {
				return err
			}
			err = m.LoadPartitions(context.Background(), collectionName, partitionNames, false)
			return
		case entity.LoadStateNotLoad:
			// partition not load
			err = m.Client.LoadPartitions(context.Background(), collectionName, partitionNames, false)
			return
		case entity.LoadStateLoading:
			// waiting partition loaded
			time.Sleep(3 * time.Second)
		}
	}
	return merr.ErrPartitionNotFullyLoaded
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
