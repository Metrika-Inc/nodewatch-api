package output

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
)

type FileOutput struct {
	path     string
	file     *os.File
	workchan chan interface{}
	writer   bufio.Writer
}

func New(path string, workChan chan interface{}) (*FileOutput, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(file)

	return &FileOutput{
		path:     path,
		file:     file,
		workchan: workChan,
		writer:   *writer,
	}, nil
}

func (f *FileOutput) Start(ctx context.Context, wg *sync.WaitGroup) {
	for {
		select {
		case <-ctx.Done():

			// If the context has been cancelled workers should be starting to shut down and the work channel will eventually be closed
			// Drain the channel of any remaining nodes updated then shutdown
			for item := range f.workchan {
				outBytes, _ := json.Marshal(item)
				outBytes = append(outBytes, []byte("\n")...)
				f.writer.Write(outBytes)
			}

			f.writer.Flush()
			f.file.Sync()
			f.file.Close()
			wg.Done()
			return
		case item := <-f.workchan:
			outBytes, _ := json.Marshal(item)
			outBytes = append(outBytes, []byte("\n")...)
			f.writer.Write(outBytes)
		}
	}
}

func (f *FileOutput) WorkChan() chan interface{} {
	return f.workchan
}
