package main

import (
	"bufio"
	"fmt"
	"github.com/Azure/go-amqp"
	"github.com/gsantomaggio/go-stream-client/pkg/stream"
	"os"
	"strconv"
	"time"
)

func main() {
	fmt.Println("Getting started with Streaming client for RabbitMQ")
	fmt.Println("Connecting to RabbitMQ streaming ...")
	var client = stream.NewStreamingClient()                                  // create Client Struct
	err := client.Connect("rabbitmq-stream://guest:guest@localhost:5551/%2f") // Connect
	if err != nil {
		fmt.Printf("Error during connection: %s", err)
		return
	}
	fmt.Println("Connected to localhost")
	streamName := "golang-stream"
	err = client.CreateStream(streamName) // Create the streaming queue
	if err != nil {
		fmt.Printf("Error creating stream: %s", err)
		return
	}

	// Get a new producer to publish the messages
	producer, err := client.NewProducer(streamName)
	if err != nil {
		fmt.Printf("Error creating producer: %s", err)
		return
	}
	numberOfMessages := 1000
	batchSize := 100

	// Create AMQP 1.0 messages, see:https://github.com/Azure/go-amqp
	// message aggregation
	var arr []*amqp.Message
	for z := 0; z < batchSize; z++ {
		arr = append(arr, amqp.NewMessage([]byte("hello stream_"+strconv.Itoa(z))))
	}
	start := time.Now()
	for z := 0; z < numberOfMessages; z++ {
		_, err = producer.BatchPublish(nil, arr) // batch send
		if err != nil {
			fmt.Printf("Error publish: %s", err)
			return
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("%d messages, published in: %s\n", numberOfMessages*batchSize, elapsed)

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Press any key to stop ")
	_, _ = reader.ReadString('\n')

	fmt.Print("Closing all producers ")
	err = client.CloseAllProducers()
	if err != nil {
		fmt.Printf("error removing producers: %s", err)
		return
	}
	err = client.DeleteStream(streamName) // Remove the streaming queue and the data
	if err != nil {
		fmt.Printf("error deleting stream: %s \n", err)
		return
	}
	fmt.Println("Bye bye")
}
