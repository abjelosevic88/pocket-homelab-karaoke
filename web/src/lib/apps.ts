// The other Pocket apps Karaoke works with.
import type { AppSlug, AppStateName } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  music: { name: 'Pocket Music', icon: 'music' }, files: { name: 'Pocket Files', icon: 'archive' }, photos: { name: 'Pocket Photos', icon: 'image' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar' }, chat: { name: 'Pocket Chat', icon: 'message' }, contacts: { name: 'Pocket Contacts', icon: 'users' },
  translate: { name: 'Pocket Translate', icon: 'globe' }, habits: { name: 'Pocket Habits', icon: 'habit' }, alerts: { name: 'Pocket Alerts', icon: 'bell' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Off', link_off: 'Link is off in Pocket Core', missing: 'Not on this box', old: 'Needs an update',
  no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not working',
}
