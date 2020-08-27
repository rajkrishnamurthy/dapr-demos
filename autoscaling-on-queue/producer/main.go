package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/google/uuid"
)

const (
	min = 1
	max = 9999
)

var (
	logger = log.New(os.Stdout, "", 0)

	brokerAddress   = getEnvVar("KAFKA_BROKER", "localhost:9092")
	topicName       = getEnvVar("KAFKA_TOPIC", "prime-requests")
	numOfThreadsStr = getEnvVar("NUMBER_OF_THREADS", "1")
)

type calcRequest struct {
	ID   string `json:"id"`
	Max  int    `json:"max"`
	Time int64  `json:"time"`
}

func main() {
	numOfThreads, err := strconv.Atoi(numOfThreadsStr)
	if err != nil || numOfThreads < 1 {
		logger.Fatalf(
			"invalid number of thread (NUMBER_OF_THREADS must be positive int): %s - %v",
			numOfThreadsStr, err,
		)
	}
	logger.Printf("number of thread: %d", numOfThreads)

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 2
	config.Producer.Return.Successes = true

	p, err := sarama.NewSyncProducer(strings.Split(brokerAddress, ","), config)
	if err != nil {
		logger.Fatalf("error creating producer: %v", err)
	}
	defer p.Close()

	stopCh := make(chan struct{})
	outCh := make(chan int64, numOfThreads)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		close(stopCh)
	}()

	for i := 1; i <= numOfThreads; i++ {
		go publish(p, outCh, stopCh)
	}

	var mux sync.Mutex
	var counter int64 = 1
	startTime := time.Now()
	tickerCh := time.NewTicker(3 * time.Second).C
	for {
		select {
		case <-outCh:
			mux.Lock()
			counter++
			mux.Unlock()
		case <-tickerCh:
			logger.Printf("%10d - %.0f/sec",
				counter, float64(counter)/time.Since(startTime).Seconds())
		case <-stopCh:
			os.Exit(0)
		}
	}
}

func publish(producer sarama.SyncProducer, outCh chan<- int64, stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		default:
			b, err := json.Marshal(calcRequest{
				ID:   uuid.New().String(),
				Max:  rand.Intn(max-min) + min,
				Time: time.Now().UTC().Unix(),
			})
			if err != nil {
				logger.Fatalf("error generating request: %v", err)
			}
			m := &sarama.ProducerMessage{
				Topic: topicName,
				Value: sarama.ByteEncoder(b),
			}
			if _, _, err := producer.SendMessage(m); err != nil {
				logger.Fatalf("error publishing request: %v", err)
			}
			outCh <- 1
		}
	}
}

func getEnvVar(key, fallbackValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(val)
	}
	return fallbackValue
}
