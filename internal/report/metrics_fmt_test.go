package report

import "testing"

func TestHumanValueMemory(t *testing.T) {
	got := HumanValue("ovnControllerMemoryAvg", 2.183e8)
	if got != "208 MiB" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanValueCPU(t *testing.T) {
	got := HumanValue("ovnCPUAvg", 0.005610)
	if got != "5.61 mCPU" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanValueLatency(t *testing.T) {
	if got := HumanValue("etcdWalFsyncP99", 0.01424); got != "14.2 ms" {
		t.Fatalf("wal %q", got)
	}
	if got := HumanValue("apiLatencyP99", 60); got != "60.00 s" {
		t.Fatalf("api p99 %q", got)
	}
}

func TestHumanLabel(t *testing.T) {
	if got := HumanLabel("apiRequestRate"); got != "API Request Rate" {
		t.Fatalf("got %q", got)
	}
}
