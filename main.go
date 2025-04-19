package main

import (
	"concurrency/clientService"
	"concurrency/common"
	"concurrency/utils"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/mailru/easygo/netpoll"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 创建epoll实例
	poller, err := netpoll.New(&netpoll.Config{})
	if err != nil {
		log.Fatalf("无法创建netpoll实例: %v", err)
	}

	// 创建频道管理器
	channelManager := clientService.NewChannelManager()
	go channelManager.Run()

	// 设置HTTP处理
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// 升级HTTP连接为WebSocket
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("WebSocket升级错误: %v", err)
			return
		}

		work, err := utils.NewEnhancedSnowflake(common.DataCenterID, common.WorkId)
		if err != nil {
			log.Fatalf("无法生成唯一ID: %v", err)
		}
		id, err := work.NextID()
		if err != nil {
			log.Fatalf("无法生成唯一ID: %v", err)
		}
		// 创建新客户端
		client := &clientService.Client{
			Id:        id,
			Conn:      conn,
			Send:      make(chan []byte, 256),
			Channels:  make(map[string]bool),
			CreatedAt: time.Now(),
		}

		// 注册客户端
		channelManager.Register <- client

		// 设置epoll处理
		desc := netpoll.Must(netpoll.HandleRead(conn))
		poller.Start(desc, func(ev netpoll.Event) {
			if ev&(netpoll.EventReadHup|netpoll.EventHup) != 0 {
				// 连接关闭
				poller.Stop(desc)
				channelManager.Unregister <- client
				conn.Close()
				return
			}

			// 处理读取事件
			go func() {
				msg, op, err := wsutil.ReadClientData(conn)
				if err != nil {
					log.Printf("读取错误: %v", err)
					poller.Stop(desc)
					channelManager.Unregister <- client
					conn.Close()
					return
				}

				// 解析消息
				var message clientService.Message
				if err := json.Unmarshal(msg, &message); err != nil {
					log.Printf("消息解析错误: %v", err)
					return
				}

				// 处理不同类型的消息
				switch message.Type {
				case "join":
					channelManager.Join <- struct {
						Client  *clientService.Client
						Channel string
					}{client, message.Channel}

				case "leave":
					channelManager.Leave <- struct {
						Client  *clientService.Client
						Channel string
					}{client, message.Channel}

				case "message":
					message.Sender = fmt.Sprintf("用户%d", client.Id)
					channelManager.Broadcast <- message
				}

				// 发送确认消息
				ack := clientService.Message{
					Type:    "ack",
					Content: "收到消息",
				}
				ackData, _ := json.Marshal(ack)
				err = wsutil.WriteServerMessage(conn, op, ackData)
				if err != nil {
					log.Printf("写入错误: %v", err)
				}
			}()
		})

		// 启动发送goroutine
		go func() {
			for message := range client.Send {
				err := wsutil.WriteServerMessage(conn, ws.OpText, message)
				if err != nil {
					log.Printf("发送错误: %v", err)
					conn.Close()
					break
				}
			}
		}()
	})

	// 创建HTTP服务器
	server := &http.Server{
		Addr: ":8080",
	}

	// 启动服务器
	go func() {
		log.Println("启动WebSocket服务器在 :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe错误: %v", err)
		}
	}()

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("关闭服务器...")

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 优雅关闭
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器强制关闭: %v", err)
	}

	log.Println("服务器优雅退出")
}
