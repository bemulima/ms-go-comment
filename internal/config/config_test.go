package config

import "testing"

func TestConfigValidateRealtimeLimits(t *testing.T) {
	t.Parallel()

	valid := Config{
		ServiceMode: "all", RealtimeTicketTTLSeconds: 30, OutboxWorkerIntervalMS: 500,
		OutboxWorkerBatch: 100, OutboxLeaseSeconds: 30, RealtimeTicketCleanupSeconds: 30,
		AccessGrantMaxTTLSeconds: 300, AccessGrantCleanupSeconds: 60,
		WSMaxConnectionsPerUser: 5, WSQueueSize: 64, WSMaxFrameBytes: 16 << 10,
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
}
