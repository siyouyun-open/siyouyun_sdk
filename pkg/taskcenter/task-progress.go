package taskcenter

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"

	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

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

type Progress struct {
	Id string `json:"id"`

	TaskType  string `json:"taskType"`
	TaskTitle string `json:"taskTitle"`
	Start     int64  `json:"start"`
	Total     int64  `json:"total"`

	Current     int64  `json:"current"`
	Percent     string `json:"percent"`
	ProcessDesc string `json:"processDesc"`

	Remain int64      `json:"remain"`
	Rate   string     `json:"rate"`
	Cost   int64      `json:"cost"`
	Extra  any        `json:"extra"`
	Status TaskStatus `json:"status"`

	mu *sync.Mutex

	ugn        *utils.UserGroupNamespace
	taskTypeDO *TaskTypeDO

	lastPublishTime   int64
	lastPublishOffset int64

	notifyTicker *time.Ticker

	rateEWMA float64
}

func (t *TaskDO) StartProgress(total int64, current ...int64) error {
	if len(current) > 0 {
		t.newProgress(total, current[0], false)
	} else {
		t.newProgress(total, 0, false)
	}
	return nil
}

func (t *TaskDO) IncrementProgress(increment int64) {
	if t.Progress == nil {
		return
	}
	now := time.Now().UnixMilli()
	var doPublish bool
	if t.taskTypeDO != nil && t.taskTypeDO.NotifyIncrement {
		t.Progress.Current += increment
		doPublish = true
	} else {
		t.Progress.calculate(increment)
		if t.Progress.notifyTicker != nil {
			if _, ok := statusForcePublish[t.Progress.Status]; ok {
				doPublish = true
			}
		} else {
			diffOffset := t.Progress.Current - t.Progress.lastPublishOffset
			doPublish = float64(diffOffset)/float64(t.Progress.Total) > t.Progress.taskTypeDO.NotifyPercentInterval
		}
	}
	if doPublish {
		Client.publish(now, t.Progress)
	}
}

func (t *TaskDO) PublishProgressDesc(notifyType NotifyType, progressDesc string) {
	if t.Progress == nil {
		return
	}
	t.Progress.ProcessDesc = progressDesc
	t.Progress.Cost = time.Now().UnixMilli() - t.Progress.Start
	Client.taskCenterImpl.PublishProgress(t.UGN, notifyType, t.Progress)
}

func (t *TaskDO) StartSubProgress(total int64, current ...int64) (string, error) {
	if t.Progress == nil || total == 0 {
		return "", errors.New("主进度不存在或子进度总数为0")
	}
	if len(current) > 0 {
		return t.newProgress(total, current[0], true), nil
	}
	return t.newProgress(total, 0, true), nil
}

func (t *TaskDO) newProgress(total, current int64, isSub bool) string {
	now := time.Now().UnixMilli()
	var percent string
	if total == 0 {
		percent = "0"
	} else {
		percent = fmt.Sprintf("%.4f", float64(current)/float64(total))
	}
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
				safeGo(func() {
					for {
						select {
						case <-t.Progress.notifyTicker.C:
							Client.publish(time.Now().UnixMilli(), t.Progress)
						case <-t.GetTaskCtx().Done():
							return
						}
					}
				})
			}
		}
	} else {
		p.mu.Lock()
		defer p.mu.Unlock()
	}
	return p.Id
}

func (p *Progress) calculate(increment int64) {
	p.Current += increment
	p.Percent = fmt.Sprintf("%.4f", float64(p.Current)/float64(p.Total))
	if p.Current >= p.Total {
		p.Status = TaskStatusSuccess
	} else {
		p.Status = TaskStatusProcessing
	}
	return
}

func (p *Progress) flush(now int64) {
	if p.Status == TaskStatusFailed || p.Status == TaskStatusCancel {
		return
	}
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
