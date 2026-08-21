import {
	clientExportRoot,
	isExecutable,
	isExecutableInPath,
	packagingRoot,
	repoRoot,
	runCmd,
} from './util.ts'
import { join } from 'node:path'
import {
	chmod,
	cp,
	mkdir,
	mkdtempDisposable,
	readFile,
	rm,
	writeFile,
} from 'node:fs/promises'
import { arch } from 'node:os'
import { createWriteStream } from 'node:fs'
import { Writable } from 'node:stream'

const appImageToolUrl: Partial<Record<NodeJS.Architecture, string>> = {
	x64: 'https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage',
	arm64: 'https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-aarch64.AppImage',
	ia32: 'https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-i686.AppImage',
} as const

export async function appImageMain(args: string[]): Promise<number> {
	let appImageToolPath = 'appimagetool'
	if (!(await isExecutableInPath(appImageToolPath))) {
		const toolDir = join(packagingRoot, 'tool')
		await mkdir(toolDir, { recursive: true })
		appImageToolPath = join(toolDir, 'appimagetool')

		if (!(await isExecutable(appImageToolPath))) {
			console.log('Missing appimagetool, will try to download it...')
			const url = appImageToolUrl[arch()]
			if (url == null) {
				throw new Error(
					`No appimagetool URL for architecture ${arch()} to download. Please install it manually.`,
				)
			}

			const urlRes = await fetch(url, {
				redirect: 'follow',
			})
			if (!urlRes.ok) {
				throw new Error(
					`Failed to download appimagetool for architecture ${arch()}: ${urlRes.status} ${urlRes.statusText}`,
				)
			}
			if (urlRes.body == null) {
				throw new Error(
					`Failed to download appimagetool for architecture ${arch()}: no body`,
				)
			}

			const writer = Writable.toWeb(createWriteStream(appImageToolPath))
			await urlRes.body.pipeTo(writer)
			await chmod(appImageToolPath, 0o755)

			console.log('Downloaded to', appImageToolPath)
		}
	}

	if (!args.includes('--no-ui')) {
		console.log('Building web UI...')
		await runCmd('make', ['webui'], repoRoot)
	}

	const arches = ['amd64', 'arm64']
	for (const arch of arches) {
		// Build the client.
		console.log(`Building client for ${arch}...`)
		await runCmd('make', [`client-linux-${arch}-noui`], repoRoot)

		console.log(`Building AppImage for ${arch}...`)

		const tmpPath = join(clientExportRoot, 'appimage')
		await mkdir(tmpPath, { recursive: true })

		const clientBinPath = join(repoRoot, 'client', 'friendnet-client')

		await rm(tmpPath, { recursive: true })
		await cp(join(packagingRoot, 'appimage'), tmpPath, {
			recursive: true,
			dereference: true,
		})
		await chmod(join(tmpPath, 'AppRun'), 0o755)
		await cp(clientBinPath, join(tmpPath, 'usr', 'bin', 'friendnet-client'))
		await chmod(join(tmpPath, 'usr', 'bin', 'friendnet-client'), 0o755)

		const pkgPath = join(
			clientExportRoot,
			`friendnet-client-linux_${arch}.AppImage`,
		)
		await runCmd(appImageToolPath, [tmpPath, pkgPath])

		await rm(tmpPath, { recursive: true })
	}

	return 0
}
