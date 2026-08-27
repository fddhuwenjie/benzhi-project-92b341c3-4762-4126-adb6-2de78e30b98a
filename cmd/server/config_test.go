package main

import "testing"

func TestValidateAddressRequiresLoopback(t *testing.T) {
	for _, value := range []string{"0.0.0.0:19081", "127.0.0.1:0", "192.168.1.1:19081"} {
		if _, err := validateAddress(value); err == nil {
			t.Fatalf("地址 %s 应被拒绝", value)
		}
	}
	if got, err := validateAddress("127.0.0.1:19081"); err != nil || got != "127.0.0.1:19081" {
		t.Fatalf("回环地址无效: %s %v", got, err)
	}
}
