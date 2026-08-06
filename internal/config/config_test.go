package config

import "testing"

func TestConfigValidateRealtimeLimits(t *testing.T) {
	t.Parallel()

	valid := Config{
		ServiceMode: "all", RealtimeTicketTTLSeconds: 30, OutboxWorkerIntervalMS: 500,
		OutboxWorkerBatch: 100, OutboxLeaseSeconds: 30, RealtimeTicketCleanupSeconds: 30,
		AccessGrantMaxTTLSeconds: 300, AccessGrantCleanupSeconds: 60,
		WSMaxConnectionsPerUser: 5, WSQueueSize: 64, WSMaxFrameBytes: 16 << 10,
		HTTPReadHeaderTimeoutSeconds: 5, HTTPReadTimeoutSeconds: 30, HTTPWriteTimeoutSeconds: 75,
		HTTPIdleTimeoutSeconds: 60, HTTPMaxHeaderBytes: 32 << 10,
		HTTPUserRateLimitRPS: 20, HTTPUserRateLimitBurst: 40, HTTPUserRateLimitMaxActors: 10_000,
		HTTPUserRateLimitIdleSeconds: 300,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
	invalidMode := valid
	invalidMode.ServiceMode = "unknown"
	if invalidMode.Validate() == nil {
		t.Fatal("invalid mode was accepted")
	}
	invalidTTL := valid
	invalidTTL.RealtimeTicketTTLSeconds = 31
	if invalidTTL.Validate() == nil {
		t.Fatal("ticket TTL above contract was accepted")
	}
	invalidGrantTTL := valid
	invalidGrantTTL.AccessGrantMaxTTLSeconds = 901
	if invalidGrantTTL.Validate() == nil {
		t.Fatal("access grant TTL above contract was accepted")
	}
	invalidHeaders := valid
	invalidHeaders.HTTPMaxHeaderBytes = 1024
	if invalidHeaders.Validate() == nil {
		t.Fatal("undersized HTTP header limit was accepted")
	}
	invalidRate := valid
	invalidRate.HTTPUserRateLimitBurst = 0
	if invalidRate.Validate() == nil {
		t.Fatal("zero HTTP rate limit burst was accepted")
	}
}
