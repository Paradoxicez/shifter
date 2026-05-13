import { useState } from 'react'
import { useRouteLoaderData } from 'react-router-dom'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { ChangePasswordDialog } from './change-password-dialog'
import type { SessionUser } from '@/lib/auth'

export default function AccountPage() {
  const { user } = useRouteLoaderData('root') as { user: SessionUser }
  const [changePwOpen, setChangePwOpen] = useState(false)

  return (
    <div className="flex flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold">Account</h1>

      <div className="flex flex-col gap-4 max-w-sm">
        <div className="flex flex-col gap-1">
          <Label>Email</Label>
          <p className="text-sm">{user.email}</p>
        </div>
        <div className="flex flex-col gap-1">
          <Label>Role</Label>
          <p className="text-sm">{user.role === 'admin' ? 'Admin' : 'Viewer'}</p>
        </div>
        <Button variant="outline" onClick={() => setChangePwOpen(true)}>
          Change password
        </Button>
      </div>

      <ChangePasswordDialog
        open={changePwOpen}
        onOpenChange={setChangePwOpen}
        onSuccess={() => toast.success('Password changed')}
      />
    </div>
  )
}
