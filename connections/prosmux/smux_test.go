package prosmux

import (
	"testing"
	"time"
)

// TestSmuxConfig_SetAsDefault 校验默认配置的固定值（plan G13）。
// 这些值与 proxy-z 客户端 19de711f 中的取值成对出现，改动任意一侧
// 都必须同步另一侧并更新本测试。
func TestSmuxConfig_SetAsDefault(t *testing.T) {
	config := &SmuxConfig{}
	config.SetAsDefault()

	if config.Mode != "fast4" {
		t.Errorf("Expected Mode 'fast4', got '%s'", config.Mode)
	}
	if config.KeepAlive != 20 {
		t.Errorf("Expected KeepAlive 20, got %d", config.KeepAlive)
	}
	if config.MTU != 1350 {
		t.Errorf("Expected MTU 1350, got %d", config.MTU)
	}
	if config.DataShard != 10 {
		t.Errorf("Expected DataShard 10, got %d", config.DataShard)
	}
	if config.ParityShard != 3 {
		t.Errorf("Expected ParityShard 3, got %d", config.ParityShard)
	}
	if config.SndWnd != 2048*2 {
		t.Errorf("Expected SndWnd %d, got %d", 2048*2, config.SndWnd)
	}
	if config.RcvWnd != 2048*2 {
		t.Errorf("Expected RcvWnd %d, got %d", 2048*2, config.RcvWnd)
	}
	if config.ScavengeTTL != 600 {
		t.Errorf("Expected ScavengeTTL 600, got %d", config.ScavengeTTL)
	}
	if config.AutoExpire != 7 {
		t.Errorf("Expected AutoExpire 7, got %d", config.AutoExpire)
	}
	if config.SmuxBuf != 16777216 {
		t.Errorf("Expected SmuxBuf %d, got %d", 16777216, config.SmuxBuf)
	}
	if config.StreamBuf != 1048576 {
		t.Errorf("Expected StreamBuf %d, got %d", 1048576, config.StreamBuf)
	}
	if config.AckNodelay != true {
		t.Errorf("Expected AckNodelay true, got %t", config.AckNodelay)
	}
	if config.SocketBuf != 4194304 {
		t.Errorf("Expected SocketBuf %d, got %d", 4194304, config.SocketBuf)
	}
}

func TestSmuxConfig_GenerateConfig(t *testing.T) {
	config := &SmuxConfig{}
	config.SetAsDefault()

	smuxConfig := config.GenerateConfig()
	if smuxConfig == nil {
		t.Fatal("Expected non-nil smux config")
	}

	if smuxConfig.MaxReceiveBuffer != 16777216 {
		t.Errorf("Expected MaxReceiveBuffer %d, got %d", 16777216, smuxConfig.MaxReceiveBuffer)
	}
	if smuxConfig.MaxStreamBuffer != 1048576 {
		t.Errorf("Expected MaxStreamBuffer %d, got %d", 1048576, smuxConfig.MaxStreamBuffer)
	}
	if smuxConfig.KeepAliveInterval != 20*time.Second {
		t.Errorf("Expected KeepAliveInterval %v, got %v", 20*time.Second, smuxConfig.KeepAliveInterval)
	}
	if smuxConfig.KeepAliveTimeout != 60*time.Second {
		t.Errorf("Expected KeepAliveTimeout %v, got %v", 60*time.Second, smuxConfig.KeepAliveTimeout)
	}
}
