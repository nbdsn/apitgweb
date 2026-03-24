package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestChannelInfoScanHandlesStringValue(t *testing.T) {
	var info ChannelInfo
	err := info.Scan(`{"is_multi_key":true,"multi_key_size":2,"multi_key_polling_index":1,"multi_key_mode":"polling"}`)
	if err != nil {
		t.Fatalf("expected string JSON to scan successfully, got error: %v", err)
	}
	if !info.IsMultiKey {
		t.Fatalf("expected IsMultiKey to be true")
	}
	if info.MultiKeySize != 2 {
		t.Fatalf("expected MultiKeySize to be 2, got %d", info.MultiKeySize)
	}
	if info.MultiKeyPollingIndex != 1 {
		t.Fatalf("expected MultiKeyPollingIndex to be 1, got %d", info.MultiKeyPollingIndex)
	}
	if info.MultiKeyMode != constant.MultiKeyModePolling {
		t.Fatalf("expected MultiKeyMode to be polling, got %q", info.MultiKeyMode)
	}
}

func TestChannelInfoScanHandlesEmptyAndNil(t *testing.T) {
	cases := []interface{}{nil, "", []byte(""), "null", []byte("null")}
	for _, value := range cases {
		var info ChannelInfo
		if err := info.Scan(value); err != nil {
			t.Fatalf("expected %#v to scan successfully, got error: %v", value, err)
		}
		if info != (ChannelInfo{}) {
			t.Fatalf("expected zero value after scanning %#v, got %#v", value, info)
		}
	}
}
