import { readdir, readFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { type MarkdownPage, readMarkdownPage } from './markdown.ts'

/**
 * A documentation section.
 * A section can technically be a standalone page with no children.
 */
export type DocSection = {
	/**
	 * The section's slug.
	 */
	slug: string

	/**
	 * The section's page, if any.
	 */
	page?: MarkdownPage

	/**
	 * The paths to static files in the section.
	 * These must be absolute paths.
	 */
	staticFilePaths: string[]

	/**
	 * Zero or more child sections.
	 */
	children: DocSection[]
}

export async function scanDirForDocHierarchy(
	dirPath: string,
): Promise<DocSection> {
	const dirAbsolute = resolve(dirPath)

	const root: DocSection = {
		slug: '',
		staticFilePaths: [],
		children: [],
	}

	const toScan: {
		dir: string
		section: DocSection
	}[] = [
		{
			dir: dirAbsolute,
			section: root,
		},
	]
	while (toScan.length > 0) {
		const { dir, section } = toScan.shift()!

		let toc: string[] | undefined = undefined

		const entries = await readdir(dir, { withFileTypes: true })
		for (const entry of entries) {
			const entryPath = join(dir, entry.name)

			if (entry.isDirectory()) {
				const child = {
					dir: entryPath,
					section: {
						slug: entry.name,
						staticFilePaths: [],
						children: [],
					},
				}
				section.children.push(child.section)
				toScan.push(child)
				continue
			}

			if (!entry.isFile()) {
				continue
			}

			const name = entry.name
			if (name.startsWith('.')) {
				continue
			}

			if (name === 'toc.txt') {
				toc = (await readFile(entryPath, 'utf8')).trim().split('\n')
				continue
			}

			if (!name.endsWith('.md')) {
				section.staticFilePaths.push(entryPath)
				continue
			}

			const page = await readMarkdownPage(entryPath)

			if (name === 'index.md' || name.startsWith('index_')) {
				section.page = await readMarkdownPage(entryPath)
				continue
			}

			section.children.push({
				slug: name.substring(0, name.length - '.md'.length),
				page: page,
				staticFilePaths: [],
				children: [],
			})
		}

		// Sort children based on table of contents.
		if (toc != null) {
			section.children.sort((a, b) => {
				const aIndex = toc.indexOf(a.slug)
				const bIndex = toc.indexOf(b.slug)
				return aIndex - bIndex
			})
		}
	}

	return root
}
