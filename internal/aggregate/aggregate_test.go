package aggregate

import "testing"

// Plan 05-02 owns the CAGG hierarchy.
func TestCAGGHierarchy(t *testing.T)       { t.Skip("Plan 05-02 Task 1: hourly→daily→monthly→yearly chain") }
func TestRefreshPolicyParams(t *testing.T) { t.Skip("Plan 05-02 Task 1: end_offset >= 2 × expected_interval_s; start_offset <= raw_retention") }
func TestRetentionPolicy(t *testing.T)     { t.Skip("Plan 05-02 Task 1: per-CAGG retention defaults match D-09") }
