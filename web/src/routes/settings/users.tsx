import { useMemo, useState } from 'react'
import { useSearchParams, Navigate } from 'react-router-dom'
import { formatDistanceToNow } from 'date-fns'
import { MoreVertical, Plus } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useCurrentUser } from '@/lib/use-current-user'
import {
  useLastAdminLookup,
  useUsersList,
  type User,
} from '@/hooks/useUsers'
import { AddUserDialog } from './AddUserDialog'
import { DisableUserDialog } from './DisableUserDialog'
import { EditUserDialog } from './EditUserDialog'
import { LogoutEverywhereDialog } from './LogoutEverywhereDialog'
import { ReEnableUserDialog } from './ReEnableUserDialog'
import { ResetPasswordDialog } from './ResetPasswordDialog'

type DialogState =
  | { kind: 'none' }
  | { kind: 'add' }
  | { kind: 'edit'; user: User }
  | { kind: 'disable'; user: User }
  | { kind: 'enable'; user: User }
  | { kind: 'reset'; user: User }
  | { kind: 'logout'; user: User }

/**
 * Plan 06-05 / UI-SPEC Surface 5 — Settings → Users page.
 *
 * Route: /settings/users. Admin-only (viewers redirect to /). Tab strip
 * lives at /settings level; this is the Users tab body.
 */
export default function UsersPage() {
  const currentUser = useCurrentUser()
  const [params, setParams] = useSearchParams()
  const showDisabled = params.get('show_disabled') === '1'

  const [dialog, setDialog] = useState<DialogState>({ kind: 'none' })

  const usersQ = useUsersList(showDisabled ? 'disabled' : 'active')
  const allUsersQ = useUsersList('active') // used for last-admin computation regardless of tab
  const isLastAdmin = useLastAdminLookup(allUsersQ.data?.users ?? [])

  // Hooks must run unconditionally before any early return.
  const orderedUsers = useMemo(() => {
    const rows = usersQ.data?.users ?? []
    if (!currentUser) return rows
    // Pin the current user to the top when present in the rendered scope.
    const selfIdx = rows.findIndex((r) => r.id === currentUser.id)
    if (selfIdx === -1) return rows
    const copy = rows.slice()
    const [self] = copy.splice(selfIdx, 1)
    return [self, ...copy]
  }, [usersQ.data, currentUser])

  if (currentUser && currentUser.role !== 'admin') {
    return <Navigate to="/" replace />
  }

  const handleShowDisabledChange = (next: boolean) => {
    const sp = new URLSearchParams(params)
    if (next) sp.set('show_disabled', '1')
    else sp.delete('show_disabled')
    setParams(sp, { replace: true })
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Users ({orderedUsers.length})</CardTitle>
          <Button onClick={() => setDialog({ kind: 'add' })}>
            <Plus className="h-4 w-4" /> Add user
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center gap-2">
            <Switch
              id="show-disabled"
              checked={showDisabled}
              onCheckedChange={handleShowDisabledChange}
            />
            <Label htmlFor="show-disabled" className="cursor-pointer">
              Show disabled
            </Label>
          </div>

          {usersQ.isLoading ? (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : orderedUsers.length === 0 ? (
            <div className="rounded-md border p-12 text-center text-sm text-muted-foreground">
              {showDisabled
                ? 'No disabled users.'
                : 'No users yet. Click “Add user” to create one.'}
            </div>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Email</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Last login</TableHead>
                    {showDisabled ? <TableHead>Status</TableHead> : null}
                    <TableHead className="w-12 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {orderedUsers.map((u) => (
                    <UserRow
                      key={u.id}
                      user={u}
                      isSelf={currentUser?.id === u.id}
                      isLastAdmin={isLastAdmin(u.id)}
                      showDisabledColumn={showDisabled}
                      onEdit={() => setDialog({ kind: 'edit', user: u })}
                      onDisable={() => setDialog({ kind: 'disable', user: u })}
                      onEnable={() => setDialog({ kind: 'enable', user: u })}
                      onReset={() => setDialog({ kind: 'reset', user: u })}
                      onLogout={() => setDialog({ kind: 'logout', user: u })}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {dialog.kind === 'add' ? (
        <AddUserDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
        />
      ) : null}
      {dialog.kind === 'edit' ? (
        <EditUserDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
          user={dialog.user}
          isSelf={currentUser?.id === dialog.user.id}
          isLastAdmin={isLastAdmin(dialog.user.id)}
        />
      ) : null}
      {dialog.kind === 'disable' ? (
        <DisableUserDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
          userId={dialog.user.id}
          name={dialog.user.name}
        />
      ) : null}
      {dialog.kind === 'enable' ? (
        <ReEnableUserDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
          userId={dialog.user.id}
          name={dialog.user.name}
        />
      ) : null}
      {dialog.kind === 'reset' ? (
        <ResetPasswordDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
          userId={dialog.user.id}
          email={dialog.user.email}
          name={dialog.user.name}
        />
      ) : null}
      {dialog.kind === 'logout' ? (
        <LogoutEverywhereDialog
          open
          onOpenChange={(next) => {
            if (!next) setDialog({ kind: 'none' })
          }}
          userId={dialog.user.id}
          name={dialog.user.name}
        />
      ) : null}
    </div>
  )
}

interface UserRowProps {
  user: User
  isSelf: boolean
  isLastAdmin: boolean
  showDisabledColumn: boolean
  onEdit: () => void
  onDisable: () => void
  onEnable: () => void
  onReset: () => void
  onLogout: () => void
}

function UserRow({
  user,
  isSelf,
  isLastAdmin,
  showDisabledColumn,
  onEdit,
  onDisable,
  onEnable,
  onReset,
  onLogout,
}: UserRowProps) {
  const rowTintClass = isSelf
    ? 'bg-primary/5'
    : isLastAdmin
      ? 'bg-secondary/40'
      : ''
  const isDisabled = user.disabled_at !== null
  return (
    <TableRow className={rowTintClass}>
      <TableCell>
        {isSelf ? <span className="font-semibold">You — </span> : null}
        {user.name}
      </TableCell>
      <TableCell>{user.email}</TableCell>
      <TableCell>
        <Badge variant={user.role === 'admin' ? 'default' : 'secondary'}>
          {user.role}
        </Badge>
      </TableCell>
      <TableCell>
        {user.last_login_at ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger className="text-left">
                {formatDistanceToNow(new Date(user.last_login_at), {
                  addSuffix: true,
                })}
              </TooltipTrigger>
              <TooltipContent>
                {new Date(user.last_login_at).toLocaleString()}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : (
          <span className="text-muted-foreground">never</span>
        )}
      </TableCell>
      {showDisabledColumn ? (
        <TableCell>
          {user.disabled_at ? (
            <Badge variant="outline">
              Disabled at {new Date(user.disabled_at).toLocaleDateString()}
            </Badge>
          ) : (
            <Badge variant="outline">Active</Badge>
          )}
        </TableCell>
      ) : null}
      <TableCell className="text-right">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label={`Actions for ${user.name}`}>
              <MoreVertical className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {isDisabled ? (
              <DropdownMenuItem onClick={onEnable}>Re-enable</DropdownMenuItem>
            ) : (
              <>
                <DropdownMenuItem onClick={onEdit}>Edit</DropdownMenuItem>
                <DropdownMenuItem onClick={onReset}>Reset password</DropdownMenuItem>
                {/*
                  D-26 client-side enforcement: hide Sign out everywhere +
                  Disable on the self row.
                */}
                {isSelf ? null : (
                  <>
                    <DropdownMenuItem onClick={onLogout}>
                      Sign out everywhere
                    </DropdownMenuItem>
                    {/* Hide Disable on the last-admin row too. */}
                    {isLastAdmin ? null : (
                      <DropdownMenuItem onClick={onDisable}>
                        Disable
                      </DropdownMenuItem>
                    )}
                  </>
                )}
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}
