import { spawn } from 'node:child_process'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'

export interface RunOptions {
  macePath?: string
  cwd?: string
}

export interface JsonOptions extends RunOptions {
  inject?: string
}

export class MaceError extends Error {
  readonly exitCode: number

  constructor(message: string, exitCode = 1) {
    super(message)
    this.name = 'MaceError'
    this.exitCode = exitCode
  }
}

export async function json(path: string, options: JsonOptions = {}): Promise<unknown> {
  const args = ['json', path]
  if (options.inject) {
    args.push('--inject', options.inject)
  }

  const output = await runMace(args, options)
  return JSON.parse(output)
}

export async function jsonText(path: string, options: JsonOptions = {}): Promise<string> {
  return runMace(buildJsonArgs(path, options), options)
}

export async function output(path: string, options: RunOptions = {}): Promise<string> {
  return runMace(['output', path], options)
}

export async function nodes(path: string, options: RunOptions = {}): Promise<string> {
  return runMace(['nodes', path], options)
}

export async function importJson(input: string, options: RunOptions = {}): Promise<string> {
  return withTempFile('input.json', input, (path) => importFile(path, options))
}

export async function importYaml(input: string, options: RunOptions = {}): Promise<string> {
  return withTempFile('input.yaml', input, (path) => importFile(path, options))
}

export async function importToml(input: string, options: RunOptions = {}): Promise<string> {
  return withTempFile('input.toml', input, (path) => importFile(path, options))
}

export async function importFile(path: string, options: RunOptions = {}): Promise<string> {
  const directory = await mkdtemp(join(tmpdir(), 'mace-node-import-'))
  try {
    const output = await runMace(['import', path, '--output-dir', directory], options)
    const lines = output.trim().split(/\r?\n/).filter(Boolean)
    const outputPath = lines.find((line) => line.endsWith('.mace'))
    if (!outputPath) {
      throw new MaceError('import did not report an output file')
    }

    return await readFile(outputPath, 'utf8')
  } finally {
    await rm(directory, { recursive: true, force: true })
  }
}

async function withTempFile(name: string, contents: string, action: (path: string) => Promise<string>): Promise<string> {
  const directory = await mkdtemp(join(tmpdir(), 'mace-node-'))
  const path = join(directory, name)

  try {
    await writeFile(path, contents, 'utf8')
    return await action(path)
  } finally {
    await rm(directory, { recursive: true, force: true })
  }
}

function buildJsonArgs(path: string, options: JsonOptions): string[] {
  const args = ['json', path]
  if (options.inject) {
    args.push('--inject', options.inject)
  }
  return args
}

async function runMace(args: string[], options: RunOptions): Promise<string> {
  const command = options.macePath ?? 'mace'

  return new Promise<string>((resolve, reject) => {
    const process = spawn(command, args, {
      cwd: options.cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
    })

    let stdout = ''
    let stderr = ''

    process.stdout.on('data', (chunk) => {
      stdout += chunk.toString()
    })
    process.stderr.on('data', (chunk) => {
      stderr += chunk.toString()
    })
    process.on('error', (error) => {
      reject(new MaceError(error.message))
    })
    process.on('close', (code) => {
      if (code === 0) {
        resolve(stdout.trim())
        return
      }

      reject(new MaceError(stderr.trim() || `mace exited with code ${code ?? 1}`, code ?? 1))
    })
  })
}
