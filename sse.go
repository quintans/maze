package maze

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"sync"
)

const eol = "\n"

// Sse is a server sent event
type Sse struct {
	Id    string
	Retry uint64
	Event string
	Data  []string
}

// NewSse creates a sse with only data
func NewSse(data ...string) Sse {
	return Sse{
		Data: data,
	}
}

type SseBroker struct {
	sync.RWMutex
	subscribers map[chan []byte]bool
	OnConnect   func() (Sse, error)
}

func NewSseBroker() *SseBroker {
	return &SseBroker{
		subscribers: make(map[chan []byte]bool),
	}
}

func (s *SseBroker) HasSubscribers() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.subscribers) > 0
}

func (s *SseBroker) subscribe(c chan []byte) {
	s.Lock()
	s.subscribers[c] = true
	s.Unlock()
}

func (s *SseBroker) unsubscribe(c chan []byte) {
	s.Lock()
	if s.subscribers[c] {
		delete(s.subscribers, c)
		close(c)
	}
	s.Unlock()
}

func write(buf bytes.Buffer, k string, v string) bytes.Buffer {
	buf.WriteString(k)
	buf.WriteString(v)
	buf.WriteString(eol)
	return buf
}

func encode(e Sse) []byte {
	var buf bytes.Buffer
	if e.Id != "" {
		buf = write(buf, "id: ", e.Id)
	}
	if e.Retry > 0 {
		buf = write(buf, "retry: ", strconv.FormatUint(e.Retry, 10))
	}
	if e.Event != "" {
		buf = write(buf, "event: ", e.Event)
	}
	if e.Data != nil {
		for _, d := range e.Data {
			buf = write(buf, "data: ", d)
		}
	}
	buf.WriteString(eol)
	return buf.Bytes()
}

func (s *SseBroker) Send(e Sse) {
	b := encode(e)

	s.Lock()
	for c := range s.subscribers {
		c <- b
	}
	s.Unlock()
}

func (s *SseBroker) Serve(c IContext) error {
	w := c.GetResponse()
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return errors.New("no flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Expires", "-1")

	sub := make(chan []byte, 1)
	if s.OnConnect != nil {
		e, err := s.OnConnect()
		if err == nil {
			sub <- encode(e)
		}
	}

	s.subscribe(sub)
	defer func() {
		s.unsubscribe(sub)
	}()

	notify := w.(http.CloseNotifier).CloseNotify()

	go func() {
		<-notify
		s.unsubscribe(sub)
	}()

	for b := range sub {
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		f.Flush()
	}
	return nil
}
