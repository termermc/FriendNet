import { readdir, readFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/**
 * Regex that documentation pages must match.
 * The first capture group is the header title.
 *
 * @example
 * ```js
 * `# Hello World
 *
 * This is a documentation page.`.match(docHeaderRegex)
 * // => ['# Hello World', 'Hello World']
 * ```
 */
const docHeaderRegex = /^#\s+(.+)/

/**
 * A documentation page.
 */
export type DocPage = {
	/**
	 * The page's title.
	 */
	title: string

	/**
	 * The markdown content.
	 */
	content: string

	/**
	 * The first paragraph in the content, or undefined if none.
	 */
	firstParagraph: string | undefined
}

/**
 * Reads a documentation page from a file.
 * @param path The path to the documentation page file.
 * @returns The documentation page.
 * @throws {Error} If the file is not a valid documentation page.
 */
export async function readDocPage(path: string): Promise<DocPage> {
	const slashIdx = path.lastIndexOf('/')
	let filename: string
	if (slashIdx === -1) {
		filename = path
	} else {
		filename = path.slice(slashIdx + 1)
	}

	// Read the file
	let content = await readFile(path, 'utf8')

	// Check if it starts with a header
	const headerMatch = content.match(docHeaderRegex)
	if (!headerMatch) {
		throw new Error(
			`Doc page ${filename} does not have a header. The first line must be a level 1 heading. Example:\n\n# Hello World`,
		)
	}

	const [, title] = headerMatch

	content = content.substring(headerMatch[0].length).trim()

	// Try to find the first paragraph.
	const nlIdx = content.indexOf('\n\n')
	let firstParagraph: string | undefined
	if (nlIdx !== -1) {
		firstParagraph = content.substring(0, nlIdx)
	}

	return {
		title,
		content,
		firstParagraph,
	}
}

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
	page?: DocPage

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

			if (!name.endsWith('.md')) {
				section.staticFilePaths.push(entryPath)
				continue
			}

			const page = await readDocPage(entryPath)

			if (name === 'index.md' || name.startsWith('index_')) {
				section.page = await readDocPage(entryPath)
				continue
			}

			section.children.push({
				slug: name.substring(0, name.length - '.md'.length),
				page: page,
				staticFilePaths: [],
				children: [],
			})
		}
	}

	return root
}
