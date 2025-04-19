package utils

import (
	"errors"
	"sync"
	"time"
)

// EnhancedSnowflake 增强版雪花算法结构体
type EnhancedSnowflake struct {
	mu            sync.Mutex
	lastTimestamp int64
	dataCenterID  int64
	workerID      int64
	sequence      int64
}

// 定义常量
const (
	workerIDBits     = int64(8) // 8位workerID，支持256个节点
	dataCenterIDBits = int64(8) // 8位datacenterID，支持256个数据中心
	sequenceBits     = int64(7) // 7位序列号，每毫秒128个序列

	maxWorkerID     = int64(-1) ^ (int64(-1) << workerIDBits)
	maxDataCenterID = int64(-1) ^ (int64(-1) << dataCenterIDBits)
	maxSequence     = int64(-1) ^ (int64(-1) << sequenceBits)

	timeShift         = workerIDBits + dataCenterIDBits + sequenceBits
	dataCenterIDShift = sequenceBits + workerIDBits
	workerIDShift     = sequenceBits
	twepoch           = int64(1609430400000) // 2021-01-01 00:00:00 +0800 CST，可自定义
)

// NewEnhancedSnowflake 创建增强版Snowflake实例
func NewEnhancedSnowflake(dataCenterID, workerID int64) (*EnhancedSnowflake, error) {
	if dataCenterID < 0 || dataCenterID > maxDataCenterID {
		return nil, errors.New("data center ID must be between 0 and 255")
	}
	if workerID < 0 || workerID > maxWorkerID {
		return nil, errors.New("worker ID must be between 0 and 255")
	}

	return &EnhancedSnowflake{
		lastTimestamp: -1,
		dataCenterID:  dataCenterID,
		workerID:      workerID,
		sequence:      0,
	}, nil
}

// NextID 生成下一个ID
func (s *EnhancedSnowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	timestamp := time.Now().UnixNano() / 1e6 // 转换为毫秒

	// 处理时钟回拨
	if timestamp < s.lastTimestamp {
		offset := s.lastTimestamp - timestamp
		if offset <= 5 { // 允许5ms内的时钟回拨，等待时间追赶
			time.Sleep(time.Duration(offset) * time.Millisecond)
			timestamp = time.Now().UnixNano() / 1e6
			if timestamp < s.lastTimestamp { // 再次检查
				return 0, errors.New("clock moved backwards after waiting")
			}
		} else {
			return 0, errors.New("clock moved backwards. Refusing to generate id")
		}
	}

	if s.lastTimestamp == timestamp {
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			// 当前毫秒序列号用完，等待下一毫秒
			for timestamp <= s.lastTimestamp {
				timestamp = time.Now().UnixNano() / 1e6
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastTimestamp = timestamp

	// 生成ID
	id := ((timestamp - twepoch) << timeShift) |
		(s.dataCenterID << dataCenterIDShift) |
		(s.workerID << workerIDShift) |
		s.sequence

	return id, nil
}

// 解析ID
func (s *EnhancedSnowflake) ParseID(id int64) (timestamp int64, dataCenterID int64, workerID int64, sequence int64) {
	timestamp = (id >> timeShift) + twepoch
	dataCenterID = (id >> dataCenterIDShift) & maxDataCenterID
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}
