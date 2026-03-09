import { createInterface } from 'node:readline/promises'
import { stdin, stdout } from 'node:process'
import { readFile, stat, unlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'

const { subtle } = globalThis.crypto

async function isFile(path: string): Promise<boolean> {
	try {
		const info = await stat(path)
		return info.isFile()
	} catch (_) {
		return false
	}
}

type UpdateInfo = {
	created_ts: number
	version: string
	description: string
	url: string
}

const goTempl = `package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   __CREATED_TS__,
	Version:     __VERSION__,
	Description: __DESC__,
	Url:         __URL__,
}
`

const updaterDir = join(import.meta.dirname, '..', 'updater', 'client')
const destUpdateFile = join(updaterDir, 'latest.json')
const destUpdateSigFile = destUpdateFile + '.sig'
const destGoFile = join(
	import.meta.dirname,
	'..',
	'..',
	'client',
	'appinfo',
	'update.go',
)

// First, check for the update file.
const tmpUpdateFilePath = '/tmp/update.json'
const tmpUpdateDefault: UpdateInfo = {
	created_ts: Math.floor(Date.now() / 1_000),
	version: '0.0.0',
	description: 'Description of this release.\nCan me multiple lines.',
	url: 'https://github.com/termermc/FriendNet/releases/tag/v0.0.0',
}
if (!(await isFile(tmpUpdateFilePath))) {
	await writeFile(
		tmpUpdateFilePath,
		JSON.stringify(tmpUpdateDefault, null, 4),
	)
	console.log(
		`Created dummy update file at ${tmpUpdateFilePath}. Go edit it and then re-run this script.`,
	)
	process.exit(0)
}

// Try to parse update file.
const updateInfo: UpdateInfo = JSON.parse(
	await readFile(tmpUpdateFilePath, 'utf8'),
)
if (
	updateInfo.version === tmpUpdateDefault.version ||
	updateInfo.description === tmpUpdateDefault.description ||
	updateInfo.url === tmpUpdateDefault.url
) {
	console.error(
		'You have not edited all the fields in the dummy update file.',
	)
	process.exit(1)
}

const MESSAGE = JSON.stringify(updateInfo)

const rl = createInterface({
	input: stdin,
	output: stdout,
})

try {
	let pkcs8B64 = (
		await rl.question(
			`Paste PKCS#8 private key WITHOUT PEM headers (no ---BEGIN PRIVATE KEY---), then press enter:`,
		)
	).trim()

	const pkcs8Der = Buffer.from(pkcs8B64, 'base64')

	const privateKey = await subtle.importKey(
		'pkcs8',
		pkcs8Der,
		{ name: 'Ed25519' },
		false,
		['sign'],
	)

	const msgBytes = new TextEncoder().encode(MESSAGE)

	const signature = await subtle.sign(
		{ name: 'Ed25519' },
		privateKey,
		msgBytes,
	)

	await writeFile(destUpdateFile, MESSAGE)
	await writeFile(
		destUpdateSigFile,
		Buffer.from(signature).toString('base64'),
	)
	await writeFile(
		destGoFile,
		goTempl
			.replace('__CREATED_TS__', JSON.stringify(updateInfo.created_ts))
			.replace('__VERSION__', JSON.stringify(updateInfo.version))
			.replace('__DESC__', JSON.stringify(updateInfo.description))
			.replace('__URL__', JSON.stringify(updateInfo.url)),
	)
	await unlink(tmpUpdateFilePath)

	console.log('Wrote latest update. Git push to announce it.')
} finally {
	rl.close()
}
