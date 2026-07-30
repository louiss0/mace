import assert from 'node:assert/strict'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { json } from '../dist/index.js'

const macePath = process.env.MACE_PATH

test('passes runtime input to the Mace CLI', async (context) => {
  assert.ok(macePath, 'set MACE_PATH to a built mace executable')

  const directory = await mkdtemp(join(tmpdir(), 'mace-node-test-'))
  context.after(() => rm(directory, { recursive: true, force: true }))

  const path = join(directory, 'runtime.mace')
  await writeFile(path, `|===|
schema Runtime: { env: string, };
|===|
[output = 'data', parse = Runtime]
{
  env: $env,
}
`)

  const result = await json(path, {
    input: '{ env: "prod", }',
    macePath,
  })

  assert.deepEqual(result, { env: 'prod' })
})
