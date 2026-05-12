import { useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'

interface ShareCredentialsPanelProps {
  email: string
  password: string
  onConfirm: () => void
}

/**
 * Plan 06-05 / D-23 — show-once credential share panel. UI-SPEC §Add User
 * dialog step 2 verbatim copy. The "I've shared this" handler purges the
 * users query from the React Query cache so the create-user response
 * (which contained the plaintext) is no longer in memory (T-06-05-06
 * mitigation).
 */
export function ShareCredentialsPanel({
  email,
  password,
  onConfirm,
}: ShareCredentialsPanelProps) {
  const qc = useQueryClient()

  const copy = (value: string, label: string) => {
    void navigator.clipboard.writeText(value).then(() => {
      toast.success(`${label} copied.`)
    })
  }

  const handleConfirm = () => {
    qc.removeQueries({ queryKey: ['users'] })
    onConfirm()
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">
        Share these credentials with the user. We won&apos;t show this password again.
      </p>

      <div className="flex flex-col gap-3">
        <CopyRow label="Email" value={email} onCopy={() => copy(email, 'Email')} />
        <CopyRow
          label="Password"
          value={password}
          mono
          onCopy={() => copy(password, 'Password')}
          dataTestId="password-block"
        />
      </div>

      <div className="flex items-start gap-2 rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 text-sm">
        <AlertTriangle aria-hidden className="mt-0.5 h-4 w-4 shrink-0 text-yellow-600" />
        <span>
          This password is shown once. After you close this dialog, only the
          user can see it (and they&apos;ll be forced to change it on first
          sign-in).
        </span>
      </div>

      <div className="flex justify-end border-t pt-4">
        <Button onClick={handleConfirm}>I&apos;ve shared this</Button>
      </div>
    </div>
  )
}

interface CopyRowProps {
  label: string
  value: string
  mono?: boolean
  onCopy: () => void
  dataTestId?: string
}

function CopyRow({ label, value, mono, onCopy, dataTestId }: CopyRowProps) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-md border bg-card px-3 py-2">
      <div className="flex flex-col">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {label}
        </span>
        <span
          data-testid={dataTestId}
          className={mono ? 'font-mono text-base' : 'text-base'}
        >
          {value}
        </span>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onCopy}
        aria-label={`Copy ${label.toLowerCase()}`}
      >
        <Copy aria-hidden className="h-4 w-4" />
      </Button>
    </div>
  )
}
