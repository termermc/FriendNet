import { argv, platform } from 'node:process'
import { debMain } from './deb.ts'

switch (platform) {
	case 'darwin':
	case 'freebsd':
	case 'linux':
	case 'netbsd':
	case 'openbsd':
		break
	default:
		console.warn(`your current platform (${platform}) is not supported by this script, it may not work!`)
}

async function main(): Promise<number> {
	const cmd = argv[2]
	if (cmd == null) {
		console.error('no command specified')
		return 1
	}

	try {
		const args = argv.slice(3)
		switch (cmd) {
			case 'deb':
				return debMain(args)
			default:
				console.error(`unknown command: ${cmd}`)
				return 1
		}
	} catch (err) {
		console.error('failed to run command:', err)
		return 1
	}
}

process.exit(await main())
