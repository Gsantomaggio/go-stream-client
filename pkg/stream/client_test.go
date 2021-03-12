package stream

import (
	"github.com/Azure/go-amqp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"strconv"
	"sync"
	"time"
)

var client *Client
var _ = BeforeSuite(func() {
	client = NewStreamingClient()
})

var _ = AfterSuite(func() {
	Expect(client.producers.Count()).To(Equal(0))
	Expect(client.responses.Count()).To(Equal(0))
})

var _ = Describe("Streaming client", func() {
	BeforeEach(func() {

	})
	AfterEach(func() {
	})

	Describe("Streaming client", func() {
		It("Connection ", func() {
			err := client.Connect("rabbitmq-stream://guest:guest@localhost:5551/%2f")
			Expect(err).NotTo(HaveOccurred())
		})
		It("Create Stream", func() {
			err := client.CreateStream("test-stream")
			Expect(err).NotTo(HaveOccurred())
		})
		It("New/Close Publisher", func() {
			producer, err := client.NewProducer("test-stream")
			Expect(err).NotTo(HaveOccurred())
			err = producer.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		It("New/Publish/Close Publisher", func() {
			producer, err := client.NewProducer("test-stream")
			Expect(err).NotTo(HaveOccurred())
			var arr []*amqp.Message
			for z := 0; z < 10; z++ {
				arr = append(arr, amqp.NewMessage([]byte("test_"+strconv.Itoa(z))))
			}
			_, err = producer.BatchPublish(nil, arr) // batch send
			Expect(err).NotTo(HaveOccurred())
			// we can't close the producer until the publish is finished
			time.Sleep(500 * time.Millisecond)
			err = producer.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		It("Multi-thread New/Publish/Close Publisher", func() {
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(wg *sync.WaitGroup) {
					defer wg.Done()
					producer, err := client.NewProducer("test-stream")
					Expect(err).NotTo(HaveOccurred())
					var arr []*amqp.Message
					for z := 0; z < 5; z++ {
						arr = append(arr, amqp.NewMessage([]byte("test_"+strconv.Itoa(z))))
					}
					_, err = producer.BatchPublish(nil, arr) // batch send
					Expect(err).NotTo(HaveOccurred())
					// we can't close the producer until the publish is finished
					time.Sleep(500 * time.Millisecond)
					err = producer.Close()
					Expect(err).NotTo(HaveOccurred())
				}(&wg)
			}
			wg.Wait()
		})

	})
})