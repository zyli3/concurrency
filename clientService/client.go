package clientService

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"
)

// 消息类型
type Message struct {
	Type    string `json:"type"`
	Channel string `json:"channel,omitempty"`
	Content string `json:"content"`
	Sender  string `json:"sender"`
}

// 客户端连接
type Client struct {
	Id        int64
	Conn      net.Conn
	Send      chan []byte
	Channels  map[string]bool
	CreatedAt time.Time
}

// 频道管理器
type ChannelManager struct {
	Clients    map[string]map[*Client]bool // 按频道归类的客户端
	Broadcast  chan Message
	Register   chan *Client
	Unregister chan *Client
	Join       chan struct {
		Client  *Client
		Channel string
	}
	Leave chan struct {
		Client  *Client
		Channel string
	}
	Mu sync.RWMutex
}

func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		Clients:    make(map[string]map[*Client]bool),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Join: make(chan struct {
			Client  *Client
			Channel string
		}),
		Leave: make(chan struct {
			Client  *Client
			Channel string
		}),
	}
}

func (cm *ChannelManager) Run() {
	for {
		select {

		case client := <-cm.Register:
			// 注册新客户端
			log.Printf("新客户端连接: %s", client.Id)

		case client := <-cm.Unregister:
			// 注销客户端，从所有频道移除
			cm.Mu.Lock()
			for channel := range client.Channels {
				delete(cm.Clients[channel], client)
			}
			cm.Mu.Unlock()
			close(client.Send)
			log.Printf("客户端断开连接: %s", client.Id)

		case join := <-cm.Join:
			// 客户端加入频道
			cm.Mu.Lock()
			if _, ok := cm.Clients[join.Channel]; !ok {
				cm.Clients[join.Channel] = make(map[*Client]bool)
			}
			cm.Clients[join.Channel][join.Client] = true
			join.Client.Channels[join.Channel] = true
			cm.Mu.Unlock()
			log.Printf("客户端 %s 加入频道: %s", join.Client.Id, join.Channel)

		case leave := <-cm.Leave:
			// 客户端离开频道
			cm.Mu.Lock()
			if _, ok := cm.Clients[leave.Channel]; ok {
				delete(cm.Clients[leave.Channel], leave.Client)
				delete(leave.Client.Channels, leave.Channel)
			}
			cm.Mu.Unlock()
			log.Printf("客户端 %s 离开频道: %s", leave.Client.Id, leave.Channel)

		case message := <-cm.Broadcast:
			// 向特定频道广播消息
			data, err := json.Marshal(message)
			if err != nil {
				log.Printf("消息序列化错误: %v", err)
				continue

			}

			cm.Mu.RLock()
			clients := cm.Clients[message.Channel]
			cm.Mu.RUnlock()

			// 向频道内所有客户端发送消息
			for client := range clients {
				select {
				case client.Send <- data:
					// 成功将消息放入客户端发送队列
				default:
					// 客户端发送队列已满，移除该客户端
					cm.Mu.Lock()
					delete(cm.Clients[message.Channel], client)
					delete(client.Channels, message.Channel)
					close(client.Send)
					cm.Mu.Unlock()
				}
			}

		}

	}
}
