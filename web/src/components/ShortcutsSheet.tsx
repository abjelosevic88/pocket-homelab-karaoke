import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'Home'], [['s'], 'Songs'], [['g'], 'Nights'], [['p'], 'Singers'], [['y'], 'History'], [[','], 'Settings']] },
  { title: 'Make', keys: [[['n'], 'Add songs']] },
  { title: 'Singing', keys: [[['space'], 'Play or pause'], [['↑ ↓'], 'Key up or down'], [['esc'], 'Back']] },
  { title: 'Big screen', keys: [[['space'], 'Play or pause'], [['N'], 'Next singer'], [['F'], 'Full screen']] },
  { title: 'Timing the words', keys: [[['space'], 'A line starts now'], [['⌫'], 'Take the last one back']] },
  { title: 'Find', keys: [[['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
