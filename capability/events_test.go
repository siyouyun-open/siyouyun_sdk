package capability

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFileEventDecodesProducerShape pins down "the event body must be decoded according to the
// producer's actual shape".
//
// The producer is the AppStore file event relay: it POSTs the platform's internal event verbatim to
// the daemon's /api/v1/bus/publish (the daemon does not re-encode fields), so subscribers receive
// exactly the shape {ugn, srcUfi, ufi, inode, action, withAvatar, payload}. Without ugn, a native
// shared instance cannot route the event to the correct user space store; without srcUfi, a rename
// event cannot find the old record.
func TestFileEventDecodesProducerShape(t *testing.T) {
	raw := json.RawMessage(`{
		"ugn": {"username": "alice", "groupName": "", "namespace": "main"},
		"srcUfi": "/ufi/pool-raw/syspool/Music/old.flac",
		"ufi": "/ufi/pool-raw/syspool/Music/new.flac",
		"inode": 4242,
		"action": 3,
		"withAvatar": true,
		"payload": {"reason": "rename"}
	}`)

	var event FileEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("decode: %v", err)
	}
	event.normalize(time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC))

	if event.Action != 3 {
		t.Fatalf("action = %d, want 3", event.Action)
	}
	if event.ActionName != "rename" {
		t.Fatalf("actionName = %q, want rename (the producer does not set it; the SDK fills it in)", event.ActionName)
	}
	if event.UFI != "/ufi/pool-raw/syspool/Music/new.flac" {
		t.Fatalf("ufi = %q", event.UFI)
	}
	if event.SrcUFI != "/ufi/pool-raw/syspool/Music/old.flac" {
		t.Fatalf("srcUfi = %q (a rename must be able to get the old UFI)", event.SrcUFI)
	}
	if event.Inode != 4242 || !event.WithAvatar {
		t.Fatalf("inode/withAvatar = %d/%v", event.Inode, event.WithAvatar)
	}
	if event.UGN.Username != "alice" || event.UGN.Namespace != "main" {
		t.Fatalf("ugn = %+v (without the user space the event cannot be routed to the correct space store)", event.UGN)
	}
	if got, want := event.Space(), "alice-main"; got != want {
		t.Fatalf("Space() = %q, want %q", got, want)
	}
	// All three fields of Scope must be carried: filling in only username would treat an event from
	// a group space as personal space.
	if scope := event.Scope(); scope.Username != "alice" || scope.Namespace != "main" {
		t.Fatalf("Scope() = %+v", scope)
	}
	if scope := (FileEvent{UGN: EventUGN{Username: "alice", GroupName: "team", Namespace: "main"}}).Scope(); scope.GroupName != "team" {
		t.Fatalf("Scope().GroupName for a group-space event = %q, want team", scope.GroupName)
	}
	if event.ReceivedAt != "2026-09-22T05:00:00Z" {
		t.Fatalf("receivedAt = %q (the producer does not set it; the SDK fills it in)", event.ReceivedAt)
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Reason != "rename" {
		t.Fatalf("payload = %s (err=%v)", event.Payload, err)
	}
}

// TestFileEventActionNames covers the readable names of the three actions, and leaves unknown
// actions empty (no invention).
func TestFileEventActionNames(t *testing.T) {
	cases := map[int]string{1: "add", 2: "delete", 3: "rename", 99: ""}
	for action, want := range cases {
		event := FileEvent{Action: action}
		event.normalize(time.Now())
		if event.ActionName != want {
			t.Fatalf("actionName for action=%d = %q, want %q", action, event.ActionName, want)
		}
		if event.ReceivedAt == "" {
			t.Fatalf("receivedAt for action=%d must not be empty", action)
		}
	}
}

// TestFileEventSpaceUsesGroupWhenPresent: a group space is {groupname}-{namespace}.
func TestFileEventSpaceUsesGroupWhenPresent(t *testing.T) {
	event := FileEvent{UGN: EventUGN{Username: "alice", GroupName: "team", Namespace: "main"}}
	if got, want := event.Space(), "team-main"; got != want {
		t.Fatalf("Space() = %q, want %q", got, want)
	}
}
