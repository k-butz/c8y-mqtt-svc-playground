package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.RFC3339,
	}))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, falling back to system environment variables")
	}

	opts := mqtt.NewClientOptions()
	brokerUrl := os.Getenv("BROKER")
	opts.AddBroker(brokerUrl)
	opts.SetClientID(os.Getenv("CLIENT_ID"))
	opts.SetUsername(os.Getenv("USERNAME"))
	opts.SetPassword(os.Getenv("PASSWORD"))
	opts.SetAutoReconnect(true)

	opts.OnConnect = func(c mqtt.Client) {
		slog.Info("Connected to MQTT Broker", "url", brokerUrl)
		token := c.Subscribe("#", 1, onMessageReceived)
		if token.Wait() && token.Error() != nil {
			slog.Error("Subscriptions failed", "err", token.Error())
		} else {
			slog.Info("Subscribed to #")
		}
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		slog.Warn("Lost connection to MQTT Broker", "err", err)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("Failed to connect", "err", token.Error())
		os.Exit(1)
	}

	// Core MQTT / Smart REST
	// upsert device, announce device capabilities and request pending operations
	client.Publish("s/us", 1, false,
		fmt.Sprintf("%s\n%s\n%s", "100,my-mqtt-device,mytype", "114,c8y_Restart", "500"))
	time.Sleep(2 * time.Second)

	// start periodically creating dummy events via Core MQTT topic
	go publishToCoreMQTT(client, 5)

	// start periodically sending messages to MQTT Service topic
	go publishToMqttSvc(client, "kobu/mqtt-svc/test", 2) // periodically send data to MQTT Service

	select {}
}

func publishToCoreMQTT(client mqtt.Client, tickerSecs int) {
	ticker := time.NewTicker(time.Duration(tickerSecs) * time.Second)
	defer ticker.Stop()

	// create a dummy event
	topic := "s/us"
	payload := "400,ping,ping"
	for range ticker.C {
		token := client.Publish(topic, 0, false, payload)
		if token.Wait() && token.Error() != nil {
			slog.Error("Publish failed", "err", token.Error())
		} else {
			slog.Info("Message sent", "topic", topic, "payload", payload)
		}
	}
}

func publishToMqttSvc(client mqtt.Client, topic string, tickerSecs int) {
	ticker := time.NewTicker(time.Duration(tickerSecs) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		payload := fmt.Sprintf(`{"time": "%s", "status": "alive"}`, time.Now().Format(time.RFC3339))
		token := client.Publish(topic, 0, false, payload)
		if token.Wait() && token.Error() != nil {
			slog.Error("Publish failed", "err", token.Error())
		} else {
			slog.Info("Message sent", "topic", topic, "payload", payload)
		}
	}
}

func onMessageReceived(client mqtt.Client, msg mqtt.Message) {
	slog.Info("Received new MQTT Message",
		"topic", msg.Topic(),
		"msgID", msg.MessageID(),
		"payload", string(msg.Payload()),
	)
}
