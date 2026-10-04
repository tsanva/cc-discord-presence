import type { Engine, Register } from 'claude-code'

// The daemon reads ~/.claude/discord-presence-module.json before the
// statusline wrapper's file, so this module is the main data source wherever
// Claude Code loads it (the desktop app, the terminal, IDE extensions). The
// statusline wrapper remains the fallback for versions without hooks modules.
const FILE = '.claude/discord-presence-module.json'

// "claude-opus-5-5" -> "Opus 5.5", the form the statusline's display_name has.
function displayName(id: string) {
  const m = id.match(/^claude-([a-z]+)-(\d+)-(\d+)/)
  return m ? `${m[1][0].toUpperCase()}${m[1].slice(1)} ${m[2]}.${m[3]}` : id
}

// HOME is usually unset on Windows; the daemon resolves the same folder.
async function filePath($: Engine) {
  const [home, userProfile] = await Promise.all([$.env.get('HOME'), $.env.get('USERPROFILE')])
  const dir = home || userProfile
  return dir ? `${dir}/${FILE}` : undefined
}

async function publish($: Engine) {
  const [path, id, cwd, root, model, usage, now] = await Promise.all([
    filePath($),
    $.session.id(),
    $.session.cwd(),
    $.session.root(),
    $.session.model(),
    $.session.usage(),
    $.clock.now(),
  ])
  if (!path) return

  // Same shape as the statusline JSON, so the daemon parses both alike.
  const data = {
    session_id: id,
    cwd,
    model: { id: model, display_name: displayName(model) },
    workspace: { current_dir: cwd, project_dir: root },
    cost: {
      total_cost_usd: usage.cost?.usd ?? 0,
      total_duration_ms: now - usage.startedAt,
    },
    // Context size: the last request's input, cached tokens included.
    context_window: { total_input_tokens: usage.context.tokens ?? 0 },
  }
  await $.fs.write(path, JSON.stringify(data))
}

// Hooks cannot delete files, so an ending session blanks the file to "{}",
// which the daemon reads as no data, but only when the file is still its own.
async function clear($: Engine) {
  const [path, id] = await Promise.all([filePath($), $.session.id()])
  if (!path) return
  const current = await $.fs.read(path).catch(() => '')
  if (current.includes(`"session_id":"${id}"`)) await $.fs.write(path, '{}')
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const result = await next(e)
    await publish($).catch(() => {})
    return result
  })

  // Discord switches to this session as soon as a prompt is sent.
  on('prompt.submit', async ($, e, next) => {
    await publish($).catch(() => {})
    return next(e)
  })

  // Keeps the figures moving during long turns; the daemon only pushes to
  // Discord when the displayed text changes.
  on('tool.call', async ($, e, next) => {
    const result = await next(e)
    await publish($).catch(() => {})
    return result
  })

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (e.agentId === undefined) await publish($).catch(() => {})
    return result
  })

  on('session.end', async ($, e, next) => {
    await clear($).catch(() => {})
    return next(e)
  })
}
