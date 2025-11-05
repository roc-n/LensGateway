package logging

import (
	"log"
	"os"

	"github.com/rs/zerolog"
)

// Global Logger Monitor, responsible for asynchronous processing and output of logs.
type Service struct {
	logChan chan *Entry
	logger  zerolog.Logger
}

func NewService(bufferSize int) *Service {
	if bufferSize <= 0 {
		bufferSize = 1024 // Default size
	}
	// Create a zerolog logger that writes compact JSON to stdout by default.
	zlogger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return &Service{
		logChan: make(chan *Entry, bufferSize),
		logger:  zlogger,
	}
}

// Start background task that continuously consumes logs from the channel.
func (s *Service) Start() {
	go func() {
		for entry := range s.logChan {
			// Choose log level based on entry.Level; default to info.
			switch entry.Level {
			case "error", "err":
				s.logger.Error().EmbedObject(entry).Msg("")
			case "warn", "warning":
				s.logger.Warn().EmbedObject(entry).Msg("")
			case "debug":
				s.logger.Debug().EmbedObject(entry).Msg("")
			default:
				s.logger.Info().EmbedObject(entry).Msg("")
			}
		}
	}()
}

// Privoides a non-blocking way to send log entries to the channel.
func (s *Service) Log(entry *Entry) {
	// select-default scheme for non-blocking send
	select {
	case s.logChan <- entry:
	default:
		// Use standard log to avoid recursion into zerolog writer
		log.Println("log channel is full, dropping log entry")
	}
}
