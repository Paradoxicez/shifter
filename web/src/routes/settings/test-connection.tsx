import { StatusRow } from '@/components/status-row'
import type { TestConnResult } from '@/lib/settings'

export interface TestConnectionPanelProps {
  result: TestConnResult | null
}

/**
 * Inline Test Connection result panel.
 *
 * UI-SPEC §"State Conventions / Test Connection result UI" — three-row
 * visual contract: gRPC StatusRow + MQTT StatusRow + (optional) explanatory
 * paragraph rendered when gRPC failed (operator must fix gRPC first).
 *
 * StatusRow itself is shared with Phase 4 SSE health and Phase 6 alert center,
 * so this panel is only the result envelope.
 */
export function TestConnectionPanel({ result }: TestConnectionPanelProps) {
  if (!result) return null
  const grpcDetail =
    result.grpc.latency_ms != null ? `${result.grpc.latency_ms} ms` : result.grpc.detail
  const mqttDetail =
    result.mqtt.latency_ms != null ? `${result.mqtt.latency_ms} ms` : result.mqtt.detail
  const grpcFailed = result.grpc.status === 'unreachable'
  return (
    <div className="rounded-md border bg-muted/30 p-4">
      <StatusRow status={result.grpc.status} label="gRPC" detail={grpcDetail} />
      <StatusRow status={result.mqtt.status} label="MQTT" detail={mqttDetail} />
      {grpcFailed ? (
        <p className="mt-3 text-sm text-muted-foreground">
          {result.grpc.detail ?? 'Verify the gRPC URL and that ChirpStack is running.'}
        </p>
      ) : null}
    </div>
  )
}
