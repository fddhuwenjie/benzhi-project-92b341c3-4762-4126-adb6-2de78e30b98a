package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func resolveAddress(flagValue string, flagWasSet bool) (string, error) {
	if flagWasSet {
		return validateAddress(flagValue)
	}
	if port := os.Getenv("PORT"); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
		}
		return fmt.Sprintf("127.0.0.1:%d", n), nil
	}
	return defaultAddress, nil
}

func validateAddress(value string) (string, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("-addr 必须为 host:port: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "", fmt.Errorf("监听地址必须明确指定非通配主机")
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("监听地址必须使用回环 IP")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("监听端口无效")
	}
	return net.JoinHostPort(host, port), nil
}
