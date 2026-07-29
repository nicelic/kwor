import { readonly, shallowRef } from 'vue'

export type ConfirmSeverity = 'info' | 'warning' | 'danger'

export interface ConfirmOptions {
  message: string
  severity: ConfirmSeverity
  confirmText: string
  cancelText?: string
  title?: string
}

interface PendingConfirm {
  options: ConfirmOptions
  resolve: (confirmed: boolean) => void
}

const pendingConfirm = shallowRef<PendingConfirm | null>(null)

export const activeConfirm = readonly(pendingConfirm)

export const confirm = (options: ConfirmOptions): Promise<boolean> => {
  // Native confirm blocks duplicate clicks. Keep the same safety property for async dialogs.
  if (pendingConfirm.value !== null) {
    return Promise.resolve(false)
  }

  return new Promise((resolve) => {
    pendingConfirm.value = { options, resolve }
  })
}

export const settleConfirm = (confirmed: boolean) => {
  const pending = pendingConfirm.value
  if (pending === null) return

  pendingConfirm.value = null
  pending.resolve(confirmed)
}

// Route changes and app teardown must not leave an async caller waiting on a
// confirmation that no longer belongs to its original screen.
export const cancelConfirm = () => {
  settleConfirm(false)
}
