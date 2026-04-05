import { access } from 'node:fs/promises'
import { constants as fsConstants } from 'node:fs'
import * as path from 'node:path'
import { spawn } from 'node:child_process'

/**
 * The root path of the repository.
 */
export const repoRoot = path.resolve(import.meta.dirname, '..')

/**
 * The root path of the packaging directory.
 */
export const packagingRoot = import.meta.dirname

/**
 * The root path of the exported client files.
 */
export const clientExportRoot = path.join(repoRoot, 'client')

export async function ensureInPath(exes: string[]) {
	for (const exe of exes) {
		if (!(await isExecutableInPath(exe))) {
			throw new Error(`executable ${exe} not found in PATH, please install it`)
		}
	}
}

/**
 * Returns whether the specified executable is in the system's PATH.
 * @param exe The executable to check.
 * @returns Whether the executable is in the system's PATH.
 */
export async function isExecutableInPath(exe: string): Promise<boolean> {
	if (!exe || exe.includes('\0')) {
		return false
	}

	// If the name includes a slash, don't consult PATH.
	if (exe.includes('/')) {
		return isExecutable(exe)
	}

	const envPath = process.env.PATH ?? ''
	if (envPath === '') {
		return false
	}

	const dirs = envPath.split(':')

	for (const dir of dirs) {
		// Empty entry means current working directory
		const baseDir = dir === '' ? process.cwd() : dir
		const fullPath = path.join(baseDir, exe)

		if (await isExecutable(fullPath)) {
			return true
		}
	}

	return false
}

async function isExecutable(filePath: string): Promise<boolean> {
	try {
		await access(filePath, fsConstants.X_OK)
		return true
	} catch {
		return false
	}
}

/**
 * Runs a command, inheriting stdout and stderr.
 * @param exe The executable to run.
 * @param args The arguments to pass to the executable.
 * @param cwd The working directory to run the command in.
 */
export function runCmd(
	exe: string,
	args: string[],
	cwd?: string,
): Promise<void> {
	return new Promise((resolve, reject) => {
		const child = spawn(exe, args, {
			stdio: 'inherit',
			shell: false,
			cwd: cwd,
		})

		child.on('error', reject)
		child.on('close', (code, signal) => {
			if (code === 0) {
				return resolve()
			}
			reject(
				new Error(
					code !== null
						? `cmd exited with code ${code}`
						: `cmd exited with signal ${signal ?? 'unknown'}`,
				),
			)
		})
	})
}
