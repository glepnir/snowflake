package snowflake

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	sequenceMask   = 1<<12 - 1
	workerMask     = 1<<5 - 1
	dataCenterMask = 1<<5 - 1

	workerLeftShift     = 12
	dataCenterLeftShift = 17
	timestampLeftShift  = 22
)

type SnowFlake struct {
	mutex sync.Mutex

	sequence     int16
	dataCenterID uint8
	workerID     uint8

	// 上次生成 ID 的时间戳（毫秒）
	lastTimestamp int64

	startTime time.Time
}

// NewWith 给定开始时间和可选的 dataCenterID 和 workerID（注意两者的顺序）
// 如果 ids 没传，则使用 machineID
func NewWith(startTime time.Time, ids ...uint8) *SnowFlake {
	var dataCenterID, workerID uint8

	idLen := len(ids)
	if idLen >= 2 {
		dataCenterID, workerID = ids[0], ids[1]
	} else if idLen == 1 {
		dataCenterID = ids[0]
		workerID = ids[0]
	} else {
		dataCenterID, workerID = machineID()
	}

	return &SnowFlake{
		startTime:    startTime.UTC(),
		dataCenterID: dataCenterID & dataCenterMask,
		workerID:     workerID & workerMask,
	}
}

func New() *SnowFlake {
	now := time.Now()
	startTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dataCenterID, workerID := machineID()
	return NewWith(startTime, dataCenterID, workerID)
}

// NextID 获取一个 ID
func (s *SnowFlake) NextID() int64 {
	now := time.Now().UTC()
	millisecond := now.UnixNano() / 1e6
	if millisecond < s.lastTimestamp {
		panic("Clock moved backwards, Refusing to generate id")
	}

	s.mutex.Lock()

	// 同一毫秒，进行毫秒内序号递增
	if millisecond == s.lastTimestamp {
		s.sequence = (s.sequence + 1) & sequenceMask
		// 当前毫秒内序号用完，堵塞到下一毫秒
		if s.sequence == 0 {
			for millisecond <= s.lastTimestamp {
				millisecond = genMillisecond()
			}
		}
	} else {
		// 时间戳改变，毫秒内序号重置
		s.sequence = 0
	}
	s.lastTimestamp = millisecond
	sequence := s.sequence

	s.mutex.Unlock()

	elaspedMillisecond := millisecond - s.startTime.UnixNano()/1e6

	return elaspedMillisecond<<timestampLeftShift |
		int64(s.dataCenterID)<<dataCenterLeftShift |
		int64(s.workerID)<<workerLeftShift |
		int64(sequence)
}

func (s *SnowFlake) String() string {
	return fmt.Sprintf("start_time:%s, data_center:%d, worker_id:%d, sequence:%d",
		s.startTime, s.dataCenterID, s.workerID, s.sequence)
}

func machineID() (uint8, uint8) {
	as, err := net.InterfaceAddrs()
	if err != nil {
		return 0, 0
	}

	for _, a := range as {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}

		ip := ipnet.IP.To4()
		if ip != nil {
			return ip[2], ip[3]
		}
	}

	return 0, 0
}

// genMillisecond 获取当前 UTC 时间的时间戳（毫秒表示）
func genMillisecond() int64 {
	return time.Now().UTC().UnixNano() / 1e6
}
