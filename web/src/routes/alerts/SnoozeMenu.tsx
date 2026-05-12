/**
 * Plan 06-04 — Snooze dropdown (UI-SPEC §Copywriting locked verbatim).
 *
 * Five items in exact order:
 *   Snooze 1 hour, Snooze 8 hours, Snooze 24 hours, Snooze 7 days,
 *   Mute until I clear
 */

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export interface SnoozeMenuProps {
  onSelect: (duration: '1h' | '8h' | '24h' | '7d' | 'mute') => void
  disabled?: boolean
}

export function SnoozeMenu({ onSelect, disabled }: SnoozeMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" disabled={disabled}>
          Snooze ▾
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => onSelect('1h')}>Snooze 1 hour</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect('8h')}>Snooze 8 hours</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect('24h')}>Snooze 24 hours</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect('7d')}>Snooze 7 days</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect('mute')}>Mute until I clear</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
