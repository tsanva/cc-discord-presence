import type { Engine, Register } from 'claude-code'

// The daemon prefers ~/.claude/discord-presence-data.json, which the
// statusline wrapper writes in the terminal. The desktop app never runs a
// statusline, so the daemon would keep showing whichever terminal session
// wrote the file last. This module writes the same file from desktop app
// sessions on start and after every turn.

// "claude-opus-5-5" -> "Opus 5.5", the form the statusline's display_name has.
function displayName(id: string) {
  const m = id.match(/^claude-([a-z]+)-(\d+)-(\d+)/)
  return m ? `${m[1][0].toUpperCase()}${m[1].slice(1)} ${m[2]}.${m[3]}` : id
}

async function publish($: Engine) {
  const [home, userProfile, id, cwd, root, model, usage, now] = await Promise.all([
    $.env.get('HOME'),
    $.env.get('USERPROFILE'),
    $.session.id(),
    $.session.cwd(),
    $.session.root(),
    $.session.model(),
    $.session.usage(),
    $.clock.now(),
  ])
  // HOME is usually unset on Windows; the daemon resolves the same folder.
  const dir = home || userProfile
  if (!dir) return

  const data = {
    session_id: id,
    cwd,
    model: { id: model, display_name: displayName(model) },
    workspace: { current_dir: cwd, project_dir: root },
    cost: {
      total_cost_usd: usage.cost?.usd ?? 0,
      total_duration_ms: now - usage.startedAt,
    },
    context_window: {
      total_input_tokens: usage.context.tokens ?? 0,
    },
  }
  await $.fs.write(`${dir}/.claude/discord-presence-data.json`, JSON.stringify(data))
}

// Only the desktop app: the terminal already feeds the file through the
// statusline with fuller figures, and other hosts (IDE extensions, headless
// `claude -p` runs, the SDK) keep the behaviour they had before.
async function isDesktopApp($: Engine) {
  return (await $.env.get('CLAUDE_CODE_ENTRYPOINT')) === 'claude-desktop'
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const result = await next(e)
    if (await isDesktopApp($)) await publish($).catch(() => {})
    return result
  })

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (e.agentId === undefined && (await isDesktopApp($))) await publish($).catch(() => {})
    return result
  })
}
