package utils

import (
	"fmt"
	"testing"
	"time"
)

func TestNewEnhancedSnowflake(t *testing.T) {
	sf, err := NewEnhancedSnowflake(1, 1)
	if err != nil {
		fmt.Println(err)
		return
	}
	// 生成并打印10个ID
	for i := 0; i < 10; i++ {
		id, err := sf.NextID()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("ID: %d\n", id)

		// 解析ID
		ts, dc, worker, seq := sf.ParseID(id)
		fmt.Printf("Timestamp: %d, DataCenterID: %d, WorkerID: %d, Sequence: %d\n",
			ts, dc, worker, seq)
		fmt.Printf("Human time: %v\n\n", time.Unix(ts/1000, 0))
	}
}
