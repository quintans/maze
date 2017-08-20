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

func (this *SseBroker) HasSubscribers() bool {
	this.RLock()
	defer this.RUnlock()
	return len(this.subscribers) > 0
}

func (this *SseBroker) subscribe(c chan []byte) {
	this.Lock()
	this.subscribers[c] = true
	this.Unlock()
}

func (this *SseBroker) unsubscribe(c chan []byte) {
	this.Lock()
	if this.subscribers[c] {
		delete(this.subscribers, c)
		close(c)
	}
	this.Unlock()
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

func (this *SseBroker) Send(e Sse) {
	var b = encode(e)

	this.Lock()
	for c := range this.subscribers {
		c <- b
	}
	this.Unlock()
}

func (this *SseBroker) Serve(c IContext) error {
	var w = c.GetResponse()
	var f, ok = w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return errors.New("No flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Expires", "-1")

	var sub = make(chan []byte, 1)
	if this.OnConnect != nil {
		var e, err = this.OnConnect()
		if err == nil {
			sub <- encode(e)
		}
	}

	this.subscribe(sub)
	defer func() {
		this.unsubscribe(sub)
	}()

	var notify = w.(http.CloseNotifier).CloseNotify()

	go func() {
		<-notify
		this.unsubscribe(sub)
	}()

	for b := range sub {
		var _, err = w.Write(b)
		if err != nil {
			return err
		}
		f.Flush()
	}
	return nil
}
