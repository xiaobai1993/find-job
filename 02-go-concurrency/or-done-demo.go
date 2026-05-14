package main

import (
	"fmt"
	"time"
)

// ============================================
// 先看不使用 or-done 会怎么写（笨拙写法）
// ============================================

func naiveWay() {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	ch3 := make(chan struct{})

	// 启动3个任务，分别在不同时间完成
	go func() { time.Sleep(1 * time.Second); close(ch1) }()
	go func() { time.Sleep(2 * time.Second); close(ch2) }()
	go func() { time.Sleep(3 * time.Second); close(ch3) }()

	// ❌ 问题：channel 数量是固定写死的，不支持动态数量
	select {
	case <-ch1:
		fmt.Println("naiveWay: ch1 先完成了")
	case <-ch2:
		fmt.Println("naiveWay: ch2 先完成了")
	case <-ch3:
		fmt.Println("naiveWay: ch3 先完成了")
	}
}

// ============================================
// or-done 模式（通用写法）
// ============================================

// orDone 输入任意个 channel，返回一个新 channel
// 只要输入中任意一个 channel 被关闭/有数据，返回的 channel 就会被关闭
func orDone(chans ...<-chan struct{}) <-chan struct{} {
	// 边界情况
	switch len(chans) {
	case 0:
		return nil
	case 1:
		return chans[0]
	}

	result := make(chan struct{})
	go func() {
		defer close(result)
		select {
		case <-chans[0]:
		case <-chans[1]:
		// ✅ 递归！把剩下的 channel 再递归处理
		case <-orDone(chans[2:]...):
		}
	}()
	return result
}

// ============================================
// or-done 使用示例
// ============================================

func smartWay() {
	// 动态创建 N 个任务 channel（数量随便改）
	taskCount := 5
	chans := make([]<-chan struct{}, 0, taskCount)

	for i := 0; i < taskCount; i++ {
		ch := make(chan struct{})
		chans = append(chans, ch)

		// 每个任务耗时不同
		go func(id int, c chan struct{}) {
			time.Sleep(time.Duration(id+1) * 500 * time.Millisecond)
			fmt.Printf("smartWay: 任务 %d 完成\n", id)
			close(c)
		}(i, ch)
	}

	// ✅ 任意一个任务完成就退出等待，不用关心具体有多少个
	fmt.Println("smartWay: 等待任意一个任务完成...")
	<-orDone(chans...)
	fmt.Println("smartWay: 有任务完成了，不等了！")
}

// ============================================
// 实际业务场景：多个数据源任意一个返回就用
// ============================================

func fetchFromDB() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second) // 模拟慢查询
		fmt.Println("数据源 1: DB 返回")
		close(ch)
	}()
	return ch
}

func fetchFromCache() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(500 * time.Millisecond) // 缓存很快
		fmt.Println("数据源 2: Cache 返回")
		close(ch)
	}()
	return ch
}

func fetchFromRemote() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(1 * time.Second)
		fmt.Println("数据源 3: 远程服务返回")
		close(ch)
	}()
	return ch
}

func businessDemo() {
	fmt.Println("\n--- 业务场景：多数据源竞速，哪个快用哪个 ---")

	result := orDone(
		fetchFromDB(),
		fetchFromCache(),
		fetchFromRemote(),
	)

	<-result
	fmt.Println("最快的数据源已经返回，可以继续处理了！")
	// 不用等其他慢的
}

// ============================================
// main
// ============================================

func main() {
	fmt.Println("=== 笨拙写法（固定数量） ===")
	naiveWay()

	fmt.Println("\n=== or-done 模式（任意数量） ===")
	smartWay()

	businessDemo()
}
