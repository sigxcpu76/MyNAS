package domain

import (
	"fmt"
	"time"
)

// Event represents a system-wide domain notification event.
type Event struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Topic     string         `json:"topic"`    // "docker", "zfs", "kvm", "lxc", "share", "system"
	Action    string         `json:"action"`   // "START", "STOP", "DIE", "CREATE", "UPDATE", "DELETE", "SCRUB"
	Resource  string         `json:"resource"` // e.g. "dockerApps/nginx", "dockerContainers/web", "zfsPools/tank"
	Message   string         `json:"message"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func NewEvent(topic, action, resource, message string) Event {
	return Event{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC(),
		Topic:     topic,
		Action:    action,
		Resource:  resource,
		Message:   message,
	}
}
