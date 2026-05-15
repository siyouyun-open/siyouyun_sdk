package taskcenter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// RateType defines the rate calculation type, determining the display format of the rate field in progress.
type RateType int

const (
	RateTypeByte RateType = iota + 1
	RateTypeUnit
	RateTypeTime
)

var statusForcePublish = map[TaskStatus]struct{}{
	TaskStatusSuccess: {},
	TaskStatusFailed:  {},
	TaskStatusCancel:  {},
}

func (rt RateType) parseRate(rate float64) string {
	switch rt {
	case RateTypeByte:
		if rate < humanize.KiByte {
			return fmt.Sprintf("%.2f B/s", rate)
		} else if rate < humanize.MiByte {
			return fmt.Sprintf("%.2f KB/s", rate/1024)
		} else if rate < humanize.GiByte {
			return fmt.Sprintf("%.2f MB/s", rate/1024/1024)
		} else {
			return fmt.Sprintf("%.2f GB/s", rate/1024/1024/1024)
		}
	case RateTypeUnit:
		return fmt.Sprintf("%.2f/s", rate)
	case RateTypeTime:
		return fmt.Sprintf("%.2fx", rate/1000)
	default:
		return fmt.Sprintf("%.2f/s", rate)
	}
}

// Progress represents task progress.
type Progress struct {
	/**
	Basic fields
	*/
	// Id is the UUID used to associate different progress events.
	Id string `json:"id"`
	// TaskType is the task type name.
	TaskType string `json:"taskType"`
	// TaskTitle is the task description.
	TaskTitle string `json:"taskTitle"`
	// Start is the start time.
	Start int64 `json:"start"`
	// Total is the total size.
	Total int64 `json:"total"`
	// Seq is the sequence number used to ensure progress update order (receiver sorts by seq).
	Seq int64 `json:"seq"`

	/**
	Real-time update fields
	*/
	// Current is the processed size.
	Current int64 `json:"current"`
	// Percent is a number less than or equal to 1, with 4 decimal places, e.g., 0.9831, 0.3100.
	Percent string `json:"percent"`
	// ProcessDesc is the task progress description; if non-empty, it is displayed alone.
	ProcessDesc string `json:"processDesc"`

	/**
	Computed fields
	*/
	// Remain is the remaining time.
	Remain int64 `json:"remain"`
	// Rate is the processing rate, e.g., 98MB/s, 5 files/s.
	Rate string `json:"rate"`
	// Cost is the elapsed time.
	Cost int64 `json:"cost"`
	// Extra is additional information.
	Extra any `json:"extra"`
	// Status is the task status.
	Status TaskStatus `json:"status"`

	/**
	Business fields
	*/
	mu *sync.Mutex
	// ugn is the namespace information.
	ugn *utils.UserGroupNamespace
	// taskTypeDO is the task type definition.
	taskTypeDO *TaskTypeDO
	// lastPublishTime is the last notification time.
	lastPublishTime int64
	// lastPublishOffset is the last notified offset.
	lastPublishOffset int64
	// notifyTicker is the notification ticker.
	notifyTicker *time.Ticker

	rateEWMA float64
}

// StartProgress starts progress tracking.
func (t *TaskDO) StartProgress(total int64, current ...int64) error {
	if len(current) > 0 {
		t.newProgress(total, current[0], false)
	} else {
		t.newProgress(total, 0, false)
	}
	return nil
}

// IncrementProgress increments the progress.
func (t *TaskDO) IncrementProgress(increment int64) {
	if t.Progress == nil {
		return
	}
	now := time.Now().UnixMilli()
	var doPublish bool
	if t.taskTypeDO != nil && t.taskTypeDO.NotifyIncrement {
		t.Progress.calculate(increment)
		doPublish = true
	} else {
		t.Progress.calculate(increment)
		if t.Progress.notifyTicker != nil {
			if _, ok := statusForcePublish[t.Progress.Status]; ok {
				doPublish = true
			}
		} else if t.Progress.taskTypeDO != nil && t.Progress.Total > 0 {
			diffOffset := t.Progress.Current - t.Progress.lastPublishOffset
			doPublish = float64(diffOffset)/float64(t.Progress.Total) > t.Progress.taskTypeDO.NotifyPercentInterval
		}
	}
	if doPublish && Client != nil {
		Client.publish(now, t.Progress)
	}
}

// PublishProgressDesc directly publishes the incremental task progress description.
func (t *TaskDO) PublishProgressDesc(notifyScope NotifyScope, progressDesc string) {
	if t.Progress == nil {
		return
	}
	t.Progress.ProcessDesc = progressDesc
	t.Progress.Cost = time.Now().UnixMilli() - t.Progress.Start
	if Client != nil {
		Client.publisher.PublishProgress(t.UGN, notifyScope, t.Progress)
	}
}

// StartSubProgress starts a sub-progress and returns the progress UUID.
func (t *TaskDO) StartSubProgress(total int64, current ...int64) (string, error) {
	if t.Progress == nil || total == 0 {
		return "", errors.New("main progress does not exist or sub-progress total is 0")
	}
	if len(current) > 0 {
		return t.newProgress(total, current[0], true), nil
	}
	return t.newProgress(total, 0, true), nil
}

// IncrementSubProgress 增加子进度
//func (t *TaskDO) IncrementSubProgress(subProgressUUID string, increment int64) {
//	if t.Progress.SubProgress == nil {
//		// 降级只推送主进度
//		t.IncrementProgress(increment)
//		return
//	}
//	if _, ok := t.Progress.SubProgress[subProgressUUID]; !ok {
//		// 降级只推送主进度
//		t.IncrementProgress(increment)
//		return
//	}
//	if t.Progress.SubProgress[subProgressUUID].Total == 0 {
//		// 降级只推送主进度
//		t.IncrementProgress(increment)
//		return
//	}
//	// 计算当前时间戳,避免notify延迟
//	now := time.Now().UnixMilli()
//	// 初始化子进度
//	t.Progress.SubProgress[subProgressUUID].calculate(increment)
//	// 同步更新主进度
//	t.Progress.calculate(increment)
//	var doPublish bool
//	if t.Progress.notifyTicker != nil {
//		if slices.Contains(statusForcePublish, t.Progress.SubProgress[subProgressUUID].Status) {
//			doPublish = true
//		}
//	} else {
//		diffOffset := t.Progress.SubProgress[subProgressUUID].Current - t.Progress.SubProgress[subProgressUUID].lastPublishOffset
//		doPublish = float64(diffOffset)/float64(t.Progress.SubProgress[subProgressUUID].Total) > t.Progress.taskTypeDO.NotifyPercentInterval
//	}
//	if doPublish {
//		Client.publish(now, t.Progress)
//	}
//}

func (t *TaskDO) newProgress(total, current int64, isSub bool) string {
	now := time.Now().UnixMilli()
	var percent string
	if total == 0 {
		percent = "0"
	} else {
		percent = fmt.Sprintf("%.4f", float64(current)/float64(total))
	}
	// The parent progress ID uses the taskDO's UUID to match the corresponding task;
	// the sub-progress ID is randomly generated.
	var progressId string
	if isSub {
		progressId = uuid.NewString()
	} else {
		progressId = t.UUID
	}
	p := &Progress{
		Id:        progressId,
		TaskType:  t.TaskType,
		TaskTitle: t.TaskTitle,
		Status:    t.Status,
		Total:     total,
		Current:   current,
		Percent:   percent,
		Start:     now,
		Cost:      0,
		Remain:    0,
		Rate:      "",
		Extra:     nil,
		Seq:       0, // Seq starts from 0 and increments
		//SubProgress: nil,

		mu:                &sync.Mutex{},
		ugn:               t.UGN,
		taskTypeDO:        t.taskTypeDO,
		lastPublishTime:   now,
		lastPublishOffset: current,
	}

	if !isSub {
		t.Progress = p
		if total != 0 && t.taskTypeDO != nil && t.taskTypeDO.NotifyTimeInterval > 0 {
			if t.Progress.notifyTicker == nil {
				t.Progress.notifyTicker = time.NewTicker(time.Duration(t.taskTypeDO.NotifyTimeInterval) * time.Millisecond)
				if t.currentCtx == nil {
					t.currentCtx = context.Background()
				}
				ctx := t.currentCtx
				utils.SafeGo(func() {
					for {
						select {
						case <-t.Progress.notifyTicker.C:
							if Client != nil {
								Client.publish(time.Now().UnixMilli(), t.Progress)
							}
						case <-ctx.Done():
							return
						}
					}
				})
			}
		}
	} else {
		p.mu.Lock()
		defer p.mu.Unlock()
		//if t.Progress.SubProgress == nil {
		//	t.Progress.SubProgress = make(map[string]*Progress)
		//}
		//t.Progress.SubProgress[p.Id] = p
	}
	return p.Id
}

// calculate computes the progress increment (thread-safe).
// Updates Current, Percent, and Seq.
func (p *Progress) ensureMu() {
	if p.mu == nil {
		p.mu = &sync.Mutex{}
	}
}

func (p *Progress) calculate(increment int64) {
	p.ensureMu()
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Current += increment
	// Fix: prevent division by zero.
	if p.Total > 0 {
		p.Percent = fmt.Sprintf("%.4f", float64(p.Current)/float64(p.Total))
	} else {
		p.Percent = "0"
	}
	if p.Current >= p.Total && p.Total > 0 {
		p.Status = TaskStatusSuccess
	} else {
		p.Status = TaskStatusProcessing
	}
	// Increment the sequence number to ensure progress update order.
	p.Seq++
}

// flush computes the progress at notification time.
// Updates Remain, Rate, Status, and Cost.
func (p *Progress) flush(now int64) {
	p.ensureMu()
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Cost = now - p.Start
	if p.taskTypeDO != nil && p.taskTypeDO.NotifyIncrement {
		if p.taskTypeDO.NotifyIncrementTotalTmpl != "" {
			p.ProcessDesc = fmt.Sprintf(p.taskTypeDO.NotifyIncrementTotalTmpl, p.Current)
		}
		return
	}
	if p.Status == TaskStatusSuccess || (p.Total > 0 && p.Current >= p.Total) {
		p.Current = p.Total
		p.Percent = "1"
		p.Remain = 0
		p.Status = TaskStatusSuccess
		p.lastPublishTime = now
		p.lastPublishOffset = p.Current
		return
	}
	if p.taskTypeDO != nil && p.taskTypeDO.NotifyRate {
		diffTime := (float64(now) - float64(p.lastPublishTime)) / 1000
		diffOffset := p.Current - p.lastPublishOffset
		instRate := float64(0)
		if diffTime > 0 {
			instRate = float64(diffOffset) / diffTime
		}
		if p.rateEWMA == 0 {
			p.rateEWMA = instRate
		} else {
			tau := float64(3)
			if diffTime <= 0 {
				diffTime = 0
			}
			decay := math.Exp(-diffTime / tau)
			p.rateEWMA = p.rateEWMA*decay + instRate*(1-decay)
		}

		rate := p.rateEWMA
		if rate > 0 {
			p.Rate = p.taskTypeDO.RateType.parseRate(rate)
			p.Remain = int64((float64(p.Total) - float64(p.Current)) / (rate))
		} else {
			p.Rate = "0"
			p.Remain = -1
		}
	}
	p.lastPublishTime = now
	p.lastPublishOffset = p.Current
}
