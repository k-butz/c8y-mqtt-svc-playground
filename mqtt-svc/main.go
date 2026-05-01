package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.RFC3339,
	}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("Service started. Press Ctrl+C to stop.")

	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, falling back to system environment variables")
	}

	serviceURL := os.Getenv("PULSAR_URL")
	tenantID := os.Getenv("TENANT_ID")
	username := os.Getenv("USERNAME")
	password := os.Getenv("PASSWORD")

	if serviceURL == "" || password == "" {
		slog.Error("Missing critical environment variables (PULSAR_URL or PASSWORD)")
		os.Exit(1)
	}

	auth, err := pulsar.NewAuthenticationBasic(fmt.Sprintf("%s/%s", tenantID, username), password)
	if err != nil {
		slog.Error("Failed to create auth", "err", err)
		os.Exit(1)
	}

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:              serviceURL,
		Authentication:   auth,
		OperationTimeout: 30 * time.Second,
	})
	if err != nil {
		slog.Error("Failed to connect to Pulsar", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	go sendCommands(ctx, client, tenantID)
	go consumeData(ctx, client, tenantID)
	<-ctx.Done()
	slog.Info("Shutting down...")
}

func sendCommands(ctx context.Context, client pulsar.Client, tenantID string) {
	// setup pulsar producer for topic to-device
	pulsarTopic := fmt.Sprintf("persistent://%s/mqtt/to-device", tenantID)
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: pulsarTopic,
	})
	if err != nil {
		slog.Error("Failed to create producer", "pulsarTopic", pulsarTopic, "err", err)
		return
	}
	defer producer.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// send messages
	// use the "Properties" and "Key" field to specify the Devices client-ID and MQTT Topic
	mqttTopic := "kobu/mqtt-svc/cmds"
	clientId := "kobu"
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload := []byte("my test message towards a device")
			_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
				Payload: payload,
				Properties: map[string]string{
					"clientID": clientId,
					"topic":    mqttTopic,
				},
				Key: clientId, // key needs to match the clientID
			})
			if err != nil {
				slog.Error("Failed to send message", "err", err)
			} else {
				slog.Info("Sent command to device",
					"pulsarTopic", pulsarTopic,
					"mqttTopic", mqttTopic,
					"payload", payload,
					"clientId", clientId)
			}
		}
	}
}

func consumeData(ctx context.Context, client pulsar.Client, tenantID string) {
	topic := fmt.Sprintf("persistent://%s/mqtt/from-device", tenantID)
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: "go-sub",
		Type:             pulsar.Shared,
	})
	if err != nil {
		slog.Error("Failed to subscribe", "topic", topic, "err", err)
		return
	}
	defer consumer.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("Worker started", "topic", topic)
	for {
		msg, err := consumer.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("Worker shutting down gracefully...")
				return
			}
			slog.Error("Error receiving message", "err", err)
			continue
		}

		slog.Info("Received new message",
			slog.Group("metadata",
				"id", msg.ID(),
				"topic", msg.Topic(),
				"key", msg.Key(),
				"producer", msg.ProducerName(),
				"eventTime", msg.EventTime(),
				"publishTime", msg.PublishTime(),
			),
			"properties", msg.Properties(),
			"payload", string(msg.Payload()),
		)

		if err := consumer.Ack(msg); err != nil {
			slog.Error("Failed to ack message", "msgID", msg.ID(), "err", err)
		}
	}
}
