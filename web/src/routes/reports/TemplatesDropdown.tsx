/**
 * TemplatesDropdown — Plan 07-11b Task 1
 *
 * Popover + Command searchable list of saved report templates.
 * Admin-only three-dot per-row Delete action with AlertDialog confirm.
 *
 * UI-SPEC Surface 6 copy verbatim.
 */

import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bookmark, MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { listTemplates, deleteTemplate, type ReportTemplate } from '@/lib/reportTemplates'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface TemplatesDropdownProps {
  currentState: object
  onLoadTemplate: (state: object) => void
  isAdmin: boolean
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TemplatesDropdown({ onLoadTemplate, isAdmin }: TemplatesDropdownProps) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [deleteCandidate, setDeleteCandidate] = useState<ReportTemplate | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const { data: templates = [] } = useQuery({
    queryKey: ['report-templates'],
    queryFn: listTemplates,
  })

  async function handleDelete() {
    if (!deleteCandidate) return
    setIsDeleting(true)
    try {
      await deleteTemplate(deleteCandidate.id)
      await qc.invalidateQueries({ queryKey: ['report-templates'] })
      setDeleteCandidate(null)
    } catch {
      toast.error('Failed to delete template')
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm" className="gap-1.5">
            <Bookmark className="h-3.5 w-3.5" />
            Templates
          </Button>
        </PopoverTrigger>
        <PopoverContent className="p-0 w-72" align="start">
          <Command>
            <CommandInput placeholder="Search templates…" />
            <CommandList>
              <CommandEmpty>
                No templates saved yet. Save the current config to create one.
              </CommandEmpty>
              {templates.map((t) => (
                <CommandItem
                  key={t.id}
                  value={t.name}
                  onSelect={() => {
                    onLoadTemplate(t.state)
                    toast.success(`Template loaded: ${t.name}`)
                    setOpen(false)
                  }}
                  className="flex items-center justify-between pr-1"
                >
                  <span className="truncate">{t.name}</span>
                  {isAdmin && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-6 w-6 shrink-0 ml-2"
                          aria-label="More options"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <MoreHorizontal className="h-3.5 w-3.5" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          variant="destructive"
                          onSelect={(e) => {
                            e.preventDefault()
                            setDeleteCandidate(t)
                            setOpen(false)
                          }}
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </CommandItem>
              ))}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      {/* Delete confirmation AlertDialog */}
      <AlertDialog
        open={!!deleteCandidate}
        onOpenChange={(o) => { if (!o) setDeleteCandidate(null) }}
      >
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete template?</AlertDialogTitle>
            <AlertDialogDescription>
              &apos;{deleteCandidate?.name}&apos; will be permanently deleted. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep template</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDelete}
              disabled={isDeleting}
            >
              Delete template
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
