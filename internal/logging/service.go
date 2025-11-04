package logging

import (
	"encoding/json"
	"log"
	"os"
)

// Global Logger Monitor, responsible for asynchronous processing and output of logs.
type Service struct {
	logChan chan *Entry
	writer  *log.Logger
}

func NewService(bufferSize int) *Service {
	if bufferSize <= 0 {
		bufferSize = 1024 // Default size
	}
	return &Service{
		// Create a buffered channel with a configurable size.
		logChan: make(chan *Entry, bufferSize),
		// Currnetly simply outputs formatted logs to standard output,
		// TODO can be replaced with file or network writer in the future.
		writer: log.New(os.Stdout, "", 0),
	}
}

// Start background task that continuously consumes logs from the channel.
func (s *Service) Start() {
	go func() {
		for entry := range s.logChan {
			// Format as json and output.
			line, err := json.Marshal(entry)
			if err != nil {
				log.Printf("failed to marshal log entry: %v", err)
				continue
			}
			s.writer.Println(string(line))
		}
	}()
}

// Privoides a non-blocking way to send log entries to the channel.
func (s *Service) Log(entry *Entry) {
	// select-default scheme for non-blocking send
	select {
	case s.logChan <- entry:
	default:
		log.Println("log channel is full, dropping log entry")
	}
}
