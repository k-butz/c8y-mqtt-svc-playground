# About

A project to test and showcase the integration between Devices and Cumulocitys MQTT Service

# Q & A

**Q1: How can I subscribe to data sent by Devices?**

A: Below code-block will receive all messages sent by any Device of your tenant. You will find the devices clientID and topic within the consumed message.

```go
// first initialize your Pulsar Client
auth, _ := pulsar.NewAuthenticationBasic(fmt.Sprintf("%s/%s", tenantID, username), password)
client, _ := pulsar.NewClient(pulsar.ClientOptions{
	URL:              pulsar+ssl://example.cumulocity.com:6651,
	Authentication:   auth,
	OperationTimeout: 30 * time.Second,
})
defer client.Close()

// then consume the messages via the persistent://{tenantid}/mqtt/from-device topic
// each message contains the clientID and topic within its metadata
consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
	Topic:            fmt.Sprintf("persistent://%s/mqtt/from-device", tenantID),
	SubscriptionName: "go-sub",
	Type:             pulsar.Shared,
})
defer consumer.Close()

for {
	msg, _ := consumer.Receive(ctx)
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
	// e.g.
	// 2026-05-01T09:59:54+02:00 INF Received new message metadata.id=3045654:722:0 metadata.topic=persistent://t2021486094/mqtt/from-device-partition-0 metadata.key=kobu metadata.producer=pulsar-226-4343284 metadata.eventTime=2026-05-01T09:59:54.733+02:00 metadata.publishTime=2026-05-01T09:59:54.733+02:00 properties="map[clientID:kobu topic:kobu/mqtt-svc/test tx.clientAuthType:BASIC tx.clientUsername:kobu]" payload="my message to MQTT Svc"

	// acknowledge the message
	if err := consumer.Ack(msg); err != nil {
		slog.Error("Failed to ack message", "msgID", msg.ID(), "err", err)
	}
}
```

**Q2: How do I send data towards Devices via MQTT Service?**

A: This code-block will send a message to a Device with clientID=kobu on topic=kobu/mqtt-svc/cmds

```go
producer, _ := client.CreateProducer(pulsar.ProducerOptions{
	Topic: fmt.Sprintf("persistent://%s/mqtt/to-device", tenantID),
})
defer producer.Close()

ticker := time.NewTicker(2 * time.Second)
defer ticker.Stop()

// send messages
// use the "Properties" and "Key" field to specify the Devices client-ID and MQTT Topic
mqttTopic := "kobu/mqtt-svc/cmds"
clientId := "kobu"
for {
	select {
	case <-ticker.C:
		payload := []byte("my test message towards a device")
		producer.Send(context.Background(), &pulsar.ProducerMessage{
			Payload: payload,
			Properties: map[string]string{
				"clientID": clientId,
				"topic":    mqttTopic,
			},
			Key: clientId, // key needs to match the mqtt clientID the Device is using
		})
		slog.Info("Sent command to device","pulsarTopic", pulsarTopic,"mqttTopic", mqttTopic,"payload", payload,"clientId", clientId)
	}
}
```

**Q3: How do I receive Operations from Core MQTT?**

A: There are two options:
1) Have a second, separate MQTT Connection. One for Port 9883 (MQTT Service), another one for 8883 (Core MQTT). All Device Operations will be sent towards the connection on Port 8883 exclusively.

2) Enable the "Smart Rest Bridge" feature on MQTT Service. Within Cumulocity Administration, there is a feature-toggle `mqtt-service.smartrest`. Once this is active, you will receive all Device Operations also via MQTT Service (port 9883). At the moment (April 2026), this is a public-preview feature and not yet in GA. Make sure to read: [https://cumulocity.com/docs/device-integration/mqtt-service/#core-mqtt-support](https://cumulocity.com/docs/device-integration/mqtt-service/#core-mqtt-support)

**Q4: What happens when I enable the Smart Rest Bridge and connect two clients with the same clientID on different ports: 9883 (MQTT Service) and 8883 (Core MQTT)?**

A: Both connections can co-exist. In this setup you would:
- Receive all Device Operations on your connections on port 8883 and 9883 in parallel
- Be able to send data via the Core MQTT Topics on both connections
- Be able to send arbitrary payloads to custom topics via the 9883 connection

