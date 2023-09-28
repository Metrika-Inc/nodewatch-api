package output

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/segmentio/kafka-go"
)

type Output struct {
	path        string
	file        *os.File
	workchan    chan interface{}
	fileWriter  bufio.Writer
	kConf       *KafkaConfig
	kafkaWriter *kafka.Writer
}

type KafkaConfig struct {
	Topic            string
	BootstrapServers []string
	Timeout          time.Duration
}

func New(path string, kafkaConfig *KafkaConfig, workChan chan interface{}) (*Output, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(file)

	output := &Output{
		path:       path,
		file:       file,
		workchan:   workChan,
		kConf:      kafkaConfig,
		fileWriter: *writer,
	}

	if kafkaConfig != nil {
		output.kafkaWriter = &kafka.Writer{
			Addr:       kafka.TCP(kafkaConfig.BootstrapServers...),
			Topic:      kafkaConfig.Topic,
			BatchBytes: 10 * 1024 * 1024, // 10MB
			// We cannot have a bigger batch because there's only one goroutine handling writes
			// Rework the output struct to have multiple goroutines handling writes or change to async mode if performance is an issue
			BatchSize:    1,
			BatchTimeout: time.Second,
			RequiredAcks: 1,
			WriteTimeout: kafkaConfig.Timeout,
		}
	}

	return output, nil
}

func (f *Output) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go f.monitorWorkChan(ctx, wg)

	for {
		select {
		case <-ctx.Done():

			// If the context has been cancelled workers should be starting to shut down and the work channel will eventually be closed
			// Drain the channel of any remaining nodes updated then shutdown
			kafkaMsgs := make([]kafka.Message, 0, len(f.workchan))
			for item := range f.workchan {
				outBytes, _ := json.Marshal(item)
				if _, err := f.fileWriter.Write(append(outBytes, byte('\n'))); err != nil {
					log.Warn("failed to write to file", log.Ctx{"error": err})
				}
				kafkaMsgs = append(kafkaMsgs, kafka.Message{Value: outBytes})
			}
			if f.kafkaWriter != nil {
				if err := f.kafkaWriter.WriteMessages(ctx, kafkaMsgs...); err != nil {
					log.Warn("failed to write to kafka", log.Ctx{"error": err})
				}
			}
			f.fileWriter.Flush()
			f.file.Sync()
			f.file.Close()
			wg.Done()
			return
		case item := <-f.workchan:
			outBytes, _ := json.Marshal(item)
			if _, err := f.fileWriter.Write(append(outBytes, byte('\n'))); err != nil {
				log.Warn("failed to write to file", log.Ctx{"error": err})
			}
			if f.kafkaWriter == nil {
				if err := f.kafkaWriter.WriteMessages(ctx, kafka.Message{Value: outBytes}); err != nil {
					log.Warn("failed to write to kafka", log.Ctx{"error": err})
				}
			}
		}
	}
}

func (f *Output) WorkChan() chan interface{} {
	return f.workchan
}

func (f *Output) monitorWorkChan(ctx context.Context, wg *sync.WaitGroup) {
	t := time.NewTicker(time.Second * 15)
	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return
		case <-t.C:
			log.Info("Workchan monitor", log.Ctx{"size": len(f.workchan)})
		}
	}
}
