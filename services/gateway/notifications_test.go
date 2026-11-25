package main

import (
	"testing"
	"time"
)

func TestNotificationSignatureAndReplayWindow(t *testing.T) {
	a := app{secret: randomToken()}
	now := time.Now()
	e := reviewEvent{Schema: 1, ID: "id", Invoice: "invoice", Timestamp: now.Unix(), State: "PENDING_REVIEW", Demo: true}
	msg, err := a.signEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.verifyEvent(msg, now); err != nil {
		t.Fatal(err)
	}
	if _, err = a.verifyEvent(msg, now.Add(301*time.Second)); err == nil {
		t.Fatal("expired event accepted")
	}
	msg.Payload += "A"
	if _, err = a.verifyEvent(msg, now); err == nil {
		t.Fatal("modified payload accepted")
	}
	e.Demo = false
	msg, _ = a.signEvent(e)
	if _, err = a.verifyEvent(msg, now); err == nil {
		t.Fatal("non-demo event accepted")
	}
}
