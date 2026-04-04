import { ensureInPath, clientExportRoot, packagingRoot, repoRoot, runCmd } from './util.ts'
import { join } from 'node:path'
import { cp, readFile, mkdir, writeFile, rm, mkdtempDisposable } from 'node:fs/promises'

export async function debMain(args: string[]): Promise<number> {
	await ensureInPath([
		'go',
		'node',
		'make',
		'tar',
		'ar',
		'fakeroot',
	])

	await rm(clientExportRoot, { recursive: true, force: true })

	if (!args.includes('--no-ui')) {
		console.log('Building web UI...')
		await runCmd('make', ['webui'], repoRoot)
	}

	const arches = ['amd64', 'arm64']
	for (const arch of arches) {
		// Build the client.
		console.log(`Building client for ${arch}...`)
		await runCmd('make', [`client-linux-${arch}-noui`], repoRoot)

		console.log(`Building deb for ${arch}...`)

		const tmpPath = join(clientExportRoot, 'deb')
		await mkdir(tmpPath, { recursive: true })

		const clientBinPath = join(repoRoot, 'client', 'friendnet-client')
		const path = join(repoRoot, 'website', 'updater', 'client', 'latest.json')
		const json = JSON.parse(await readFile(path, 'utf-8'))
		const version = json.version

		const controlPath = join(tmpPath, 'DEBIAN', 'control')

		await cp(join(packagingRoot, 'deb'), tmpPath, { recursive: true })
		await cp(join(packagingRoot, 'fs'), tmpPath, { recursive: true })
		await cp(clientBinPath, join(tmpPath, 'usr', 'bin', 'friendnet-client'))
		await runCmd('chmod', ['0755', join(tmpPath, 'usr', 'bin', 'friendnet-client')])

		// Replace placeholders in control file
		{
			const control = await readFile(controlPath, 'utf-8')
			await writeFile(
				controlPath,
				control
					.replaceAll('$VERSION', version)
					.replaceAll('$ARCH', arch),
			)
		}

		const pkgPath = join(clientExportRoot, `friendnet-client-linux_${arch}.deb`)
		await using buildTmp = await mkdtempDisposable('/tmp/fn-packaging')
		const debBinaryPath = join(buildTmp.path, 'debian-binary')
		const controlTarPath = join(buildTmp.path, 'control.tar.gz')
		const dataTarPath = join(buildTmp.path, 'data.tar.gz')

		const fakerootScript = `
			printf "2.0\\n" > ${JSON.stringify(debBinaryPath)}
			tar -C DEBIAN --owner=0 --group=0 --numeric-owner -czf ${JSON.stringify(controlTarPath)} .
			tar -C . --exclude=./DEBIAN --owner=0 --group=0 --numeric-owner -czf ${JSON.stringify(dataTarPath)} .
			ar rcs ${JSON.stringify(pkgPath)} ${JSON.stringify(debBinaryPath)} ${JSON.stringify(controlTarPath)} ${JSON.stringify(dataTarPath)}
		`
		await runCmd('fakeroot', ['sh', '-c', fakerootScript], tmpPath)
		await rm(tmpPath, { recursive: true })
	}

	return 0
}
