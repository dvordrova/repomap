package telemetry

import (
	"log"
	"sync"
	"time"
)

type Event struct {
	Integration string
	Operation   string
	Outcome     string
	OrderID     string
	At          time.Time
}

type Recorder interface {
	Record(Event)
}

type Logger struct{}

func (Logger) Record(event Event) {
	log.Printf("integration=%s operation=%s outcome=%s order=%s", event.Integration, event.Operation, event.Outcome, event.OrderID)
}

type Memory struct {
	mu     sync.Mutex
	events []Event
}

func (m *Memory) Record(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *Memory) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}
