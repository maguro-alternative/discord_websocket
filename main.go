package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var errr = godotenv.Load()

var TOKEN = os.Getenv("TOKEN")

type DiscordGatewayPayload struct {
	Op      int             `json:"op"`
	Data    json.RawMessage `json:"d,omitempty"`
	Seq     *int            `json:"s,omitempty"`
	Event   *string         `json:"t,omitempty"`
	ErrCode int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
}
type DiscordGatewayIdentity struct {
	Token      string `json:"token"`
	Properties struct {
		OS      string `json:"$os"`
		Browser string `json:"$browser"`
		Device  string `json:"$device"`
	} `json:"properties"`
	Intents int `json:"intents"`
}

type DiscordGatewayHeartbeat struct {
	Op  int `json:"op"`
	D   int `json:"d"`
}

func main() {
	// DiscordのGateway URLを取得する
	gatewayURL, err := getGatewayURL()
	if err != nil {
		log.Fatal("getGatewayURL error:", err)
	}

	// WebSocket接続を開始する
	wsConn, _, err := websocket.DefaultDialer.Dial(gatewayURL, nil)
	if err != nil {
		log.Fatal("websocket dial error:", err)
	}
	defer wsConn.Close()

	// WebSocketからのメッセージを読み取る
	go func() {
		for {
			var payload DiscordGatewayPayload
			err := wsConn.ReadJSON(&payload)
			if err != nil {
				log.Println("read error:", err)
				// 再接続を試みる
				for {
					log.Println("reconnecting...")
					wsConn, _, err = websocket.DefaultDialer.Dial(gatewayURL, nil)
					if err != nil {
						log.Println("websocket dial error:", err)
						time.Sleep(time.Second)
						continue
					}
					break
				}
				continue
			}
			fmt.Printf("recv: %#v\n", payload)

			// 受信したOpCodeが10 (Hello) の場合はHeartbeatを開始する
			if payload.Op == 10 {
				var data struct {
					HeartbeatInterval int `json:"heartbeat_interval"`
				}
				if err := json.Unmarshal(payload.Data, &data); err != nil {
					log.Println("unmarshal error:", err)
					return
				}
				log.Printf("starting heartbeat (interval=%dms)", data.HeartbeatInterval)
				go func() {
					for {
						time.Sleep(time.Duration(data.HeartbeatInterval) * time.Millisecond)
						if err := wsConn.WriteJSON(DiscordGatewayHeartbeat{Op: 1, D: 42}); err != nil {
							log.Println("write error:", err)
							return
						}
						log.Println("sent heartbeat")
					}
				}()
			}
		}
	}()

	// Discordセッションに接続情報を送信する
	identity := DiscordGatewayIdentity{
		Token: TOKEN,
		Properties: struct {
			OS      string `json:"$os"`
			Browser string `json:"$browser"`
			Device  string `json:"$device"`
		}{
			OS:      "linux",
			Browser: "my_library",
			Device:  "my_library",
		},
		Intents: 32767,	//intentをすべて有効にしている
	}
	sendGatewayMessage(wsConn, 2, identity)

	// SIGINTまたはSIGTERMが送信されるまで待機する
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

func getGatewayURL() (string, error) {
	resp, err := http.Get("https://discord.com/api/gateway")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		URL string `json:"url"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s?encoding=json&v=9", result.URL), nil
}

var sendMutex sync.Mutex

func sendGatewayMessage(conn *websocket.Conn, op int, data interface{}) error {
	sendMutex.Lock()
	defer sendMutex.Unlock()

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	message := struct {
		Op int             `json:"op"`
		D  json.RawMessage `json:"d"`
	}{
		Op: op,
		D:  payload,
	}

	return conn.WriteJSON(message)
}
