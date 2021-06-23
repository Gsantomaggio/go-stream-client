package main

import (
	"bufio"
	"fmt"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/ha"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

func CheckErr(err error) {
	if err != nil {
		fmt.Printf("%s ", err)
		os.Exit(1)
	}
}

var idx = 0

func CreateArrayMessagesForTesting(bacthMessages int) []message.StreamMessage {
	var arr []message.StreamMessage
	for z := 0; z < bacthMessages; z++ {
		idx++
		arr = append(arr, amqp.NewMessage([]byte(strconv.Itoa(idx))))
	}
	return arr
}

func handlePublishConfirm(confirms stream.ChannelPublishConfirm) {
	var counter int32 = 0
	go func() {
		for messagesIds := range confirms {
			for _, m := range messagesIds {
				if !m.Confirmed {
					if atomic.AddInt32(&counter, 1)%10 == 0 {
						fmt.Printf("Confirmed %s message - status %t - %d \n  ", m.Message.GetData(), m.Confirmed, atomic.LoadInt32(&counter))
					}
				}
			}
		}
	}()
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("HA producer example")
	fmt.Println("Connecting to RabbitMQ streaming ...")

	env, err := stream.NewEnvironment(
		stream.NewEnvironmentOptions().
			SetHost("localhost").
			SetPort(5552).
			SetUser("guest").
			SetPassword("guest"))
	CheckErr(err)

	streamName := "test"
	err = env.DeclareStream(streamName,
		&stream.StreamOptions{
			MaxLengthBytes: stream.ByteCapacity{}.GB(2),
		},
	)

	rProducer, err := ha.NewHAProducer(env, streamName, nil)
	CheckErr(err)

	chPublishConfirm := rProducer.NotifyPublishConfirmation()
	handlePublishConfirm(chPublishConfirm)
	time.Sleep(4 * time.Second)
	for i := 0; i < 1000000; i++ {
		err := rProducer.BatchPublish(CreateArrayMessagesForTesting(10))
		time.Sleep(500 * time.Millisecond)
		if i%1000 == 0 {
			fmt.Println("sent.. " + strconv.Itoa(i))
		}
		CheckErr(err)
	}

	fmt.Println("Press any key to start consuming ")
	_, _ = reader.ReadString('\n')

	handleMessages := func(consumerContext stream.ConsumerContext, message *amqp.Message) {
		fmt.Printf("messages consumed: %s \n ", message.Data)
	}

	consumer, err := env.NewConsumer(streamName,
		handleMessages,
		stream.NewConsumerOptions().
			SetConsumerName("my_consumer"))
	CheckErr(err)

	fmt.Println("Press any key to stop ")
	_, _ = reader.ReadString('\n')
	time.Sleep(200 * time.Millisecond)
	err = rProducer.Close()
	CheckErr(err)
	err = consumer.Close()
	CheckErr(err)
	err = env.DeleteStream(streamName)
	CheckErr(err)
	err = env.Close()
	CheckErr(err)
}
