import { panelBaseURL } from './api'

type LoginNavigator = () => Promise<void>

let loginNavigator: LoginNavigator | null = null
let pendingLoginNavigation: Promise<void> | null = null
let pageReloadingForLogin = false

export const registerLoginNavigator = (navigator: LoginNavigator) => {
  loginNavigator = navigator
}

export const requestLoginNavigation = async (): Promise<void> => {
  if (pendingLoginNavigation) return pendingLoginNavigation

  const operation = (async () => {
    if (!loginNavigator) return
    try {
      await loginNavigator()
    } catch {
      // A concurrent navigation or an unavailable session probe must not
      // leave an unhandled rejection in an expired-session response path.
    }
  })()

  pendingLoginNavigation = operation
  try {
    await operation
  } finally {
    if (pendingLoginNavigation === operation) {
      pendingLoginNavigation = null
    }
  }
}

export const reloadToLogin = () => {
  if (pageReloadingForLogin || typeof window === 'undefined') return
  pageReloadingForLogin = true
  const loginURL = new URL(`${panelBaseURL}login`, window.location.origin)
  window.location.replace(loginURL.href)
}
