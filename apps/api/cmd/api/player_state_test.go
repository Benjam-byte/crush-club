package main

import (
	"testing"
	"time"
)

func TestCanExcludeLobbyPlayerImmediatelyWhenDisconnected(t *testing.T) {
	disconnectedAt := time.Now().UTC()
	testCases := []struct {
		name           string
		isHost         bool
		connected      bool
		disconnectedAt *time.Time
		want           bool
	}{
		{name: "freshly disconnected player", disconnectedAt: &disconnectedAt, want: true},
		{name: "connected player", connected: true, disconnectedAt: &disconnectedAt},
		{name: "player without disconnect marker"},
		{name: "disconnected host", isHost: true, disconnectedAt: &disconnectedAt},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := canExcludeLobbyPlayer(testCase.isHost, testCase.connected, testCase.disconnectedAt); got != testCase.want {
				t.Fatalf("canExcludeLobbyPlayer() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestReconnectGracePeriodRemainsReservedForHostTransfer(t *testing.T) {
	if reconnectGracePeriod != 90*time.Second {
		t.Fatalf("reconnectGracePeriod = %s, want 90s", reconnectGracePeriod)
	}
}
