/**
 * CodecTestRunner — Surface 4 (plan 07-08)
 *
 * Collapsible panel embedded in the device-profile editor below the codec_js
 * textarea. Sends hex + fPort to POST /api/device-profiles/{id}/test-codec and
 * renders decoded JSON + canonical mapping in a dual-tab output pane, or a red
 * error panel on goja exceptions.
 *
 * Accordion note: @radix-ui/react-accordion is not installed in this project.
 * Collapsible behaviour is implemented with a simple button+state toggle to
 * avoid adding a dependency for a single usage (Rule 3 deviation from plan).
 * The "Stack trace" sub-section inside the error panel uses the same pattern.
 */

import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Copy, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { JsonTree } from '@/components/metering-point/JsonTree'
import { runCodecTest, type CodecTestResponse } from '@/lib/codecTest'

export interface CodecTestRunnerProps {
  profileId: string
}

export function CodecTestRunner({ profileId }: CodecTestRunnerProps) {
  const [expanded, setExpanded] = useState(false)
  const [fPort, setFPort] = useState<string>('')
  const [hex, setHex] = useState('')
  const [result, setResult] = useState<CodecTestResponse | null>(null)

  const mutation = useMutation({
    mutationFn: () =>
      runCodecTest(profileId, hex.trim(), fPort.trim() ? parseInt(fPort, 10) : 0),
    onSuccess: (data) => {
      setResult(data)
    },
  })

  return (
    <div className="rounded-md border">
      {/* Collapsible header */}
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-semibold hover:bg-secondary/50"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
        Test Codec
      </button>

      {/* Expanded body */}
      {expanded && (
        <div className="border-t p-4">
          <div className="flex flex-col gap-6 md:flex-row">
            {/* Left / input pane */}
            <div className="flex flex-col gap-4 md:w-1/2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="codec-test-fport">fPort (optional)</Label>
                <Input
                  id="codec-test-fport"
                  type="number"
                  min={0}
                  max={255}
                  placeholder="e.g. 1"
                  value={fPort}
                  onChange={(e) => setFPort(e.target.value)}
                  className="w-28"
                />
              </div>

              <div className="flex flex-col gap-1.5">
                <Label htmlFor="codec-test-hex">Payload (hex bytes)</Label>
                <Textarea
                  id="codec-test-hex"
                  rows={4}
                  className="font-mono"
                  placeholder="e.g. 6F 01 23 45 AB CD"
                  value={hex}
                  onChange={(e) => setHex(e.target.value)}
                />
              </div>

              <div className="flex flex-col gap-2">
                <Button
                  type="button"
                  onClick={() => mutation.mutate()}
                  disabled={!hex.trim() || mutation.isPending}
                  className="w-fit"
                >
                  <Play className="mr-2 h-4 w-4" />
                  Run Test
                </Button>
                <p className="text-sm text-muted-foreground">
                  Results are not saved. Clear the form to reset.
                </p>
              </div>
            </div>

            {/* Right / output pane */}
            <div className="flex-1">
              {result?.error_message ? (
                <ErrorPanel result={result} />
              ) : result?.decoded_json ? (
                <OutputTabs result={result} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  Run a test to see output here.
                </p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Output tabs
// ---------------------------------------------------------------------------

function OutputTabs({ result }: { result: CodecTestResponse }) {
  return (
    <Tabs defaultValue="decoded">
      <TabsList>
        <TabsTrigger value="decoded" role="tab">Decoded JSON</TabsTrigger>
        <TabsTrigger value="canonical" role="tab">Canonical Mapping</TabsTrigger>
      </TabsList>

      <TabsContent value="decoded">
        <div className="relative rounded-md border p-3">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-2 top-2"
            aria-label="Copy decoded JSON to clipboard"
            title="Copy as JSON"
            onClick={() =>
              navigator.clipboard.writeText(
                JSON.stringify(result.decoded_json, null, 2),
              )
            }
          >
            <Copy className="h-4 w-4" />
          </Button>
          <div className="pr-10">
            <JsonTree value={result.decoded_json} defaultOpen />
          </div>
        </div>
      </TabsContent>

      <TabsContent value="canonical">
        <div className="relative rounded-md border p-3">
          {result.canonical_mapping == null ? (
            <p className="text-sm text-muted-foreground">
              No canonical mapping available — this profile has no mappings
              configured, or the decoded fields didn&apos;t match any mapping
              target. Configure mappings in the right pane to see canonical
              output here.
            </p>
          ) : (
            <>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-2 top-2"
                aria-label="Copy canonical mapping to clipboard"
                title="Copy as JSON"
                onClick={() =>
                  navigator.clipboard.writeText(
                    JSON.stringify(result.canonical_mapping, null, 2),
                  )
                }
              >
                <Copy className="h-4 w-4" />
              </Button>
              <div className="pr-10">
                <JsonTree value={result.canonical_mapping} defaultOpen />
              </div>
            </>
          )}
        </div>
      </TabsContent>
    </Tabs>
  )
}

// ---------------------------------------------------------------------------
// Error panel
// ---------------------------------------------------------------------------

function ErrorPanel({ result }: { result: CodecTestResponse }) {
  // Stack trace visible by default when present so operator doesn't need extra click
  const [stackExpanded, setStackExpanded] = useState(true)

  return (
    <div
      role="alert"
      className="rounded-md border border-destructive/30 bg-destructive/10 p-4"
    >
      <div className="text-xs font-semibold uppercase tracking-wide text-destructive">
        Error
      </div>
      <div className="mt-1 text-sm">{result.error_message}</div>

      {result.error_line != null && result.error_col != null && (
        <div className="mt-1 font-mono text-sm text-muted-foreground">
          at line {result.error_line}, column {result.error_col}
        </div>
      )}

      {result.error_stack && (
        <div className="mt-3">
          <button
            type="button"
            aria-expanded={stackExpanded}
            onClick={() => setStackExpanded((v) => !v)}
            className="flex items-center gap-1 text-xs font-semibold hover:underline"
          >
            {stackExpanded ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
            Stack trace
          </button>

          {stackExpanded && (
            <div className="mt-2 flex flex-col gap-2">
              <textarea
                readOnly
                className="w-full rounded-md border bg-secondary font-mono text-xs"
                rows={6}
                value={result.error_stack}
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                aria-label="Copy error stack trace to clipboard"
                onClick={() =>
                  navigator.clipboard.writeText(result.error_stack ?? '')
                }
              >
                Copy Stack Trace
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
