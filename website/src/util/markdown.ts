import { readFile } from 'node:fs/promises'

/**
 * Regex that markdown pages must match.
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
const mdHeaderRegex = /^#\s+(.+)/

/**
 * A markdown page.
 */
export type MarkdownPage = {
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
 * Reads a markdown page from a file.
 * @param path The path to the markdown page file.
 * @returns The markdown page.
 * @throws {Error} If the file is not a valid markdown page.
 */
export async function readMarkdownPage(path: string): Promise<MarkdownPage> {
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
	const headerMatch = content.match(mdHeaderRegex)
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
